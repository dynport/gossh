package gossh

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func New(host, user string) (c *Client) {
	return &Client{
		User: user,
		Host: host,
	}
}

type Client struct {
	User        string
	Host        string
	Port        int
	Agent       net.Conn
	password    string
	Conn        *ssh.Client
	DebugWriter Writer
	ErrorWriter Writer
	InfoWriter  Writer
	PrivateKey  string
}

func (c *Client) Password(user string) (password string, e error) {
	if c.password != "" {
		return c.password, nil
	}
	return "", fmt.Errorf("password must be set with SetPassword()")
}

func (c *Client) Close() {
	if c.Conn != nil {
		c.Conn.Close()
	}
	if c.Agent != nil {
		c.Agent.Close()
	}
}

func (client *Client) Attach() error {
	options := []string{"-o", "UserKnownHostsFile=/dev/null", "-o", "StrictHostKeyChecking=no"}
	if client.User != "" {
		options = append(options, "-l", client.User)
	}
	options = append(options, client.Host)
	cmd := exec.Command("/usr/bin/ssh", options...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	return cmd.Run()
}

func (c *Client) SetPassword(password string) {
	c.password = password
}

func (c *Client) SetPrivateKey(privateKey string) {
	c.PrivateKey = privateKey
}

func (c *Client) Connection() (*ssh.Client, error) {
	if c.Conn != nil {
		return c.Conn, nil
	}
	e := c.Connect()
	if e != nil {
		return nil, e
	}
	return c.Conn, nil
}

func (c *Client) ConnectWhenNotConnected() (e error) {
	if c.Conn != nil {
		return nil
	}
	return c.Connect()
}

func (c *Client) Connect() (err error) {
	if c.Port == 0 {
		c.Port = 22
	}
	config := &ssh.ClientConfig{
		User:            c.User,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	keys := []ssh.Signer{}
	if c.password != "" {
		config.Auth = append(config.Auth, ssh.Password(c.password))
	}
	if c.Agent, err = net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
		signers, err := agent.NewClient(c.Agent).Signers()
		if err == nil {
			keys = append(keys, signers...)
		}
	}

	if len(c.PrivateKey) != 0 {
		if pk, err := readPrivateKey(c.PrivateKey); err == nil {
			keys = append(keys, pk)
		}
	} else {
		if pk, err := readPrivateKey(os.ExpandEnv("$HOME/.ssh/id_rsa")); err == nil {
			keys = append(keys, pk)
		}
	}

	if len(keys) > 0 {
		config.Auth = append(config.Auth, ssh.PublicKeys(keys...))
	}

	c.Conn, err = ssh.Dial("tcp", fmt.Sprintf("%s:%d", c.Host, c.Port), config)
	return err
}

func readPrivateKey(path string) (ssh.Signer, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKey(b)
}

func (c *Client) Execute(s string) (r *Result, e error) {
	started := time.Now()
	if e = c.ConnectWhenNotConnected(); e != nil {
		return nil, e
	}
	ses, e := c.Conn.NewSession()
	if e != nil {
		return nil, e
	}
	defer ses.Close()

	tmodes := ssh.TerminalModes{
		53:  0,     // disable echoing
		128: 14400, // input speed = 14.4kbaud
		129: 14400, // output speed = 14.4kbaud
	}

	if e := ses.RequestPty("xterm", 80, 40, tmodes); e != nil {
		return nil, e
	}

	r = &Result{
		StdoutBuffer: &LogWriter{LogTo: c.Debug},
		StderrBuffer: &LogWriter{LogTo: c.Error},
	}

	ses.Stdout = r.StdoutBuffer
	ses.Stderr = r.StderrBuffer
	c.Info(fmt.Sprintf("[EXEC  ] %s", s))
	r.Error = ses.Run(s)
	c.Info(fmt.Sprintf("=> %.06f", time.Now().Sub(started).Seconds()))
	if exitError, ok := r.Error.(*ssh.ExitError); ok {
		r.ExitStatus = exitError.ExitStatus()
	}
	r.Runtime = time.Now().Sub(started)
	if !r.Success() {
		r.Error = fmt.Errorf("process exited with %d", r.ExitStatus)
	}
	return r, r.Error
}

func (c *Client) Debug(args ...interface{}) {
	c.Write(c.DebugWriter, args)
}

func (c *Client) Error(args ...interface{}) {
	c.Write(c.ErrorWriter, args)
}

func (c *Client) Info(args ...interface{}) {
	c.Write(c.InfoWriter, args)
}

var b64 = base64.StdEncoding

func (c *Client) WriteFile(path, content, owner string, mode int) (res *Result, e error) {
	return c.Execute(c.WriteFileCommand(path, content, owner, mode))
}

func (c *Client) WriteFileCommand(path, content, owner string, mode int) string {
	buf := &bytes.Buffer{}
	zipper := gzip.NewWriter(buf)
	zipper.Write([]byte(content))
	zipper.Flush()
	zipper.Close()
	encoded := b64.EncodeToString(buf.Bytes())
	hash := sha256.New()
	hash.Write([]byte(content))
	checksum := fmt.Sprintf("%x", hash.Sum(nil))
	tmpPath := "/tmp/gossh." + checksum
	dir := filepath.Dir(path)
	cmd := fmt.Sprintf("sudo mkdir -p %s && echo %s | base64 -d | gunzip | sudo tee %s", dir, encoded, tmpPath)
	if owner != "" {
		cmd += " && sudo chown " + owner + " " + tmpPath
	}
	if mode > 0 {
		cmd += fmt.Sprintf(" && sudo chmod %o %s", mode, tmpPath)
	}
	cmd = cmd + " && sudo mv " + tmpPath + " " + path
	return cmd
}

func (c *Client) Write(writer Writer, args []interface{}) {
	if writer != nil {
		writer(args...)
	}
}

// Returns an HTTP client that sends all requests through the SSH connection (aka tunnelling).
func NewHttpClient(sshClient *Client) (httpClient *http.Client, e error) {
	if e = sshClient.ConnectWhenNotConnected(); e != nil {
		return nil, e
	}
	httpClient = &http.Client{}
	httpClient.Transport = &http.Transport{Proxy: http.ProxyFromEnvironment, Dial: sshClient.Conn.Dial}
	return httpClient, nil
}

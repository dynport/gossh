package gossh

import (
	"bytes"
	"code.google.com/p/go.crypto/ssh"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
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
	Conn        *ssh.ClientConn
	DebugWriter Writer
	ErrorWriter Writer
	InfoWriter  Writer
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
	log.Printf("executing %#v", options)
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

func (c *Client) ConnectWhenNotConnected() (e error) {
	if c.Conn != nil {
		return nil
	}
	return c.Connect()
}

func (c *Client) Connect() (e error) {
	if c.Port == 0 {
		c.Port = 22
	}
	var auths []ssh.ClientAuth

	if c.password != "" {
		auths = append(auths, ssh.ClientAuthPassword(c))
	}

	if c.Agent, e = net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); e == nil {
		auths = append(auths, ssh.ClientAuthAgent(ssh.NewAgentClient(c.Agent)))
	}

	config := &ssh.ClientConfig{
		User: c.User,
		Auth: auths,
	}
	c.Conn, e = ssh.Dial("tcp", fmt.Sprintf("%s:%d", c.Host, c.Port), config)
	if e != nil {
		return e
	}
	return nil
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
	r = &Result{
		StdoutBuffer: &LogWriter{LogTo: c.Debug},
		StderrBuffer: &LogWriter{LogTo: c.Error},
	}

	ses.Stdout = r.StdoutBuffer
	ses.Stderr = r.StderrBuffer
	c.Info(fmt.Sprintf("[EXEC  ] %s", s))
	r.Error = ses.Run(s)
	c.Info(fmt.Sprintf("=> %.06f", time.Now().Sub(started).Seconds()))
	ses.Close()
	if exitError, ok := r.Error.(*ssh.ExitError); ok {
		r.ExitStatus = exitError.ExitStatus()
	}
	r.Runtime = time.Now().Sub(started)
	if !r.Success() {
		e = r.Error
	}
	return r, e
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

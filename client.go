package gossh

import (
	"code.google.com/p/go.crypto/ssh"
	"fmt"
	"net"
	"os"
	"time"
)

func New(host, user string) (c *Client) {
	c = &Client{
		User: user,
		Host: host,
	}
	return
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

func (c *Client) SetPassword(password string) {
	c.password = password
}

func (c *Client) ConnectWhenNotConnected() (e error) {
	if c.Conn != nil {
		return
	}
	return c.Connect()
}

func (c *Client) Connect() (e error) {
	if c.Port == 0 {
		c.Port = 22
	}
	c.Debug("connecting " + c.Host)
	var auths []ssh.ClientAuth

	if c.password != "" {
		auths = append(auths, ssh.ClientAuthPassword(c))
	}

	if c.Agent, e = net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); e == nil {
		auths = append(auths, ssh.ClientAuthAgent(ssh.NewAgentClient(c.Agent)))
	}

	config := &ssh.ClientConfig{
		User: "root",
		Auth: auths,
	}
	c.Conn, e = ssh.Dial("tcp", fmt.Sprintf("%s:%d", c.Host, c.Port), config)
	if e != nil {
		return
	}
	return
}

func (c *Client) Execute(s string) (r *Result, e error) {
	started := time.Now()
	e = c.ConnectWhenNotConnected()
	if e != nil {
		return
	}
	ses, e := c.Conn.NewSession()
	if e != nil {
		return
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
	return
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

func (c *Client) Write(writer Writer, args []interface{}) {
	if writer != nil {
		writer(args...)
	}
}

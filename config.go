package gossh

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type Config struct {
	Host     string
	User     string
	Port     int
	Password string
}

func (c *Config) Connection() (*ssh.Client, error) {
	port := c.Port
	if port == 0 {
		port = 22
	}

	var auths []ssh.AuthMethod
	if c.Password != "" {
		auths = append(auths, ssh.Password(c.Password))
	} else if sshAgent, e := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); e == nil {
		auths = append(auths, ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers))
	}

	config := &ssh.ClientConfig{
		User: c.User,
		Auth: auths,
	}
	return ssh.Dial("tcp", fmt.Sprintf("%s:%d", c.Host, port), config)
}

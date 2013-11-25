# gossh

Golang ssh library

## Example
    package main

    import (
      "github.com/dynport/gossh"
      "log"
    )

    // returns a function of type gossh.Writer func(...interface{})
    // MakeLogger just adds a prefix (DEBUG, INFO, ERROR)
    func MakeLogger(prefix string) gossh.Writer {
      return func(args ...interface{}) {
        log.Println((append([]interface{}{prefix}, args...))...)
      }
    }

    func main() {
      client := gossh.New("some.host", "user")
      // my default agent authentication is used. use
      // client.SetPassword("<secret>")
      // for password authentication
      client.DebugWriter = MakeLogger("DEBUG")
      client.InfoWriter = MakeLogger("INFO ")
      client.ErrorWriter = MakeLogger("ERROR")

      defer client.Close()
      rsp, e := client.Execute("uptime")
      if e != nil {
        client.ErrorWriter(e.Error())
      }
      client.InfoWriter(rsp.String())

      rsp, e = client.Execute("echo -n $(cat /proc/loadavg); cat /does/not/exists")
      if e != nil {
        client.ErrorWriter(e.Error())
        client.ErrorWriter("STDOUT: " + rsp.Stdout())
        client.ErrorWriter("STDERR: " + rsp.Stderr())
      }
    }

Prints this result:

    2013/08/25 00:31:40 DEBUG connecting some.host
    2013/08/25 00:31:41 INFO  [EXEC  ] uptime
    2013/08/25 00:31:41 DEBUG 22:31:41 up 375 days, 10:44,  0 users,  load average: 0.09, 0.13, 0.22
    2013/08/25 00:31:41 INFO  => 0.944143
    2013/08/25 00:31:41 INFO  map[stdout:72 bytes stderr:0 bytes runtime:0.944202 status:0]
    2013/08/25 00:31:41 DEBUG already connected
    2013/08/25 00:31:41 INFO  [EXEC  ] echo -n $(cat /proc/loadavg); cat /does/not/exists
    2013/08/25 00:31:41 DEBUG 0.09 0.13 0.22 1/455 23396
    2013/08/25 00:31:41 ERROR cat: /does/not/exists
    2013/08/25 00:31:41 ERROR : No such file or directory
    2013/08/25 00:31:41 INFO  => 0.067075
    2013/08/25 00:31:41 ERROR Process exited with: 1. Reason was:  ()
    2013/08/25 00:31:41 ERROR STDOUT: 0.09 0.13 0.22 1/455 23396
    2013/08/25 00:31:41 ERROR STDERR: cat: /does/not/exists: No such file or directory

## Tunnelling HTTP Connections
For services not bound to the public interface of a machine, tunnelling is a quite nice SSH feature. It allows to use a
remote service like it is running at the local machine. This concept is used in the HTTP client returned by the
NewHttpClient function. It is a common net/http.Client, but all requests are sent through the SSH connection given.

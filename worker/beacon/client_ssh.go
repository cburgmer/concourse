package beacon

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"code.cloudfoundry.org/lager"
	"golang.org/x/crypto/ssh"
)

var ErrAllGatewaysUnreachable = errors.New("all worker SSH gateways unreachable")

func NewSSHClient(logger lager.Logger, config Config) Client {
	return &sshClient{
		logger: logger,
		config: config,
	}
}

type sshClient struct {
	logger lager.Logger
	config Config

	client  *ssh.Client
	tcpConn net.Conn
}

func (c *sshClient) Dial() (Closeable, error) {
	var err error
	tsaAddr, conn, err := c.tryDialAll()
	if err != nil {
		c.logger.Error("failed-to-connect-to-any-tsa", err)
		return nil, err
	}

	c.tcpConn = conn

	var pk ssh.Signer

	if c.config.TSAConfig.WorkerPrivateKey != nil {
		pk, err = ssh.NewSignerFromKey(c.config.TSAConfig.WorkerPrivateKey.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to construct ssh public key from worker key: %s", err)
		}
	} else {
		return nil, fmt.Errorf("public worker key is not provided")
	}

	clientConfig := &ssh.ClientConfig{
		User: "beacon", // doesn't matter

		HostKeyCallback: c.config.checkHostKey,

		Auth: []ssh.AuthMethod{ssh.PublicKeys(pk)},
	}

	clientConn, chans, reqs, err := ssh.NewClientConn(c.tcpConn, tsaAddr, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to construct client connection: %s", err)
	}

	c.client = ssh.NewClient(clientConn, chans, reqs)

	return c.client, nil
}

func (c *sshClient) KeepAlive() (<-chan error, chan<- struct{}) {
	logger := c.logger.Session("keepalive")

	errs := make(chan error, 1)

	kas := time.NewTicker(5 * time.Second)
	cancel := make(chan struct{})

	go func() {
		for {
			// ignore reply; server may just not have handled it, since there's no
			// standard keepalive request name

			_, _, err := c.client.Conn.SendRequest("keepalive", true, []byte("sup"))
			if err != nil {
				logger.Error("failed", err)
				errs <- err
				return
			}

			select {
			case <-kas.C:
			case <-cancel:
				//errs <- nil
				conn, ok := c.tcpConn.(*net.TCPConn)
				if !ok {
					return
				}

				if err := conn.SetKeepAlive(false); err != nil {
					logger.Error("cancel-failed", err)
					return
				}

				return
			}
		}
	}()

	return errs, cancel
}

func (c *sshClient) NewSession(stdin io.Reader, stdout io.Writer, stderr io.Writer) (Session, error) {
	sess, err := c.client.NewSession()
	if err != nil {
		return nil, err
	}

	sess.Stdin = stdin
	sess.Stdout = stdout
	sess.Stderr = stderr

	return sess, nil
}

func (c *sshClient) Proxy(from, to string) error {
	remoteListener, err := c.client.Listen("tcp", from)
	if err != nil {
		return fmt.Errorf("failed to listen remotely: %s", err)
	}
	go c.proxyListenerTo(remoteListener, to)
	return nil
}

func (c *sshClient) tryDialAll() (string, net.Conn, error) {
	hosts := map[string]struct{}{}
	for _, host := range c.config.TSAConfig.Host {
		hosts[host] = struct{}{}
	}

	for host, _ := range hosts {
		conn, err := keepaliveDialer("tcp", host, 10*time.Second, c.config.Registration.RebalanceTime)
		if err != nil {
			c.logger.Error("failed-to-connect-to-tsa", err)
			continue
		}

		return host, conn, nil
	}

	return "", nil, ErrAllGatewaysUnreachable
}

func (c *sshClient) proxyListenerTo(listener net.Listener, addr string) {
	for {
		rConn, err := listener.Accept()
		if err != nil {
			break
		}

		go c.handleForwardedConn(rConn, addr)
	}
}

func (c *sshClient) handleForwardedConn(rConn net.Conn, addr string) {
	defer func() {
		c.logger.Info("XXX-closing-via-handle-forwarded-conn")
		rConn.Close()
	}()

	var lConn net.Conn
	for {
		var err error
		lConn, err = net.Dial("tcp", addr)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}

		break
	}

	wg := new(sync.WaitGroup)

	pipe := func(to io.WriteCloser, from io.ReadCloser) {
		// if either end breaks, close both ends to ensure they're both unblocked,
		// otherwise io.Copy can block forever if e.g. reading after write end has
		// gone away
		defer to.Close()
		defer from.Close()
		defer wg.Done()

		io.Copy(to, from)
	}

	wg.Add(1)
	go pipe(lConn, rConn)

	wg.Add(1)
	go pipe(rConn, lConn)

	wg.Wait()
}

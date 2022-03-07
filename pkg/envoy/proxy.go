package envoy

import (
	"net"
	"os"
	"os/exec"
)

type ProxyConfig struct {
	Filename           string
	Node               string
	LogLevel           string
	ComponentLogLevel  string
	NodeIPs            []string
	DNSRefreshRate     string
	PodName            string
	PodNamespace       string
	PodIP              net.IP
	SDSUDSPath         string
	SDSTokenPath       string
	STSPort            int
	ControlPlaneAuth   bool
	DisableReportCalls bool
	OutlierLogPath     string
	PilotCertProvider  string
	StatNameLength     string
	ServiceCluster     string
}

type proxy struct {
	ProxyConfig
	extraArgs []string
}

// Proxy defines command interface for a proxy
type Proxy interface {
	// IsLive returns true if the server is up and running (i.e. past initialization).
	//	IsLive() bool

	// Run command for a config, epoch, and abort channel
	Run(interface{}, int, <-chan error) error

	// Drains the current epoch.
	//	Drain() error

	// Cleanup command for an epoch
	//	Cleanup(int)
}

// NewProxy creates an instance of the proxy control commands
func NewProxy(cfg ProxyConfig) Proxy {
	// inject tracing flag for higher levels
	var args []string
	if cfg.LogLevel != "" {
		args = append(args, "-l", cfg.LogLevel)
	}
	if cfg.ComponentLogLevel != "" {
		args = append(args, "--component-log-level", cfg.ComponentLogLevel)
	}

	return &proxy{
		ProxyConfig: cfg,
		extraArgs:   args,
	}
}

func (e *proxy) Run(config interface{}, epoch int, abort <-chan error) error {
	// spin up a new Envoy process
	args := e.args(e.Filename, epoch)

	/* #nosec */
	cmd := exec.Command("/usr/local/bin/envoy", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-abort:
		if errKill := cmd.Process.Kill(); errKill != nil {
		}

		return err
	case err := <-done:
		return err
	}
}

func (e *proxy) args(fname string, epoch int) []string {
	return []string{"-c", fname}
	/**
	proxyLocalAddressType := "v4"
	startupArgs := []string{"-c", fname,
		"--restart-epoch", fmt.Sprint(epoch),
		"--service-cluster", e.ServiceCluster,
		"--service-node", e.Node,
		"--max-obj-name-len", fmt.Sprint(e.StatNameLength),
		"--local-address-ip-version", proxyLocalAddressType,
		"--log-format", fmt.Sprintf("[Envoy (Epoch %d)] ", epoch) + "[%Y-%m-%d %T.%e][%t][%l][%n] %v",
	}

	startupArgs = append(startupArgs, e.extraArgs...)

	return startupArgs
	**/
}

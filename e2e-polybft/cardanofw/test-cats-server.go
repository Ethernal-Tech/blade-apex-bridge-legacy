package cardanofw

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

const TestCatsServerAPIKey = "zum-zum-eprom"

type TestCatsServerConfig struct {
	ID          int
	ConfigPath  string
	LogsPath    string
	NetworkType wallet.CardanoNetworkType
	Port        int
	SocketPath  string
	StdOut      io.Writer
}

type TestCatsServer struct {
	config *TestCatsServerConfig
	node   *framework.Node
}

func NewTestCatsServer(config *TestCatsServerConfig) (*TestCatsServer, error) {
	srv := &TestCatsServer{
		config: config,
	}

	return srv, srv.Start()
}

func (t *TestCatsServer) IsRunning() bool {
	return t.node != nil
}

func (t *TestCatsServer) Stop() error {
	if err := t.node.Stop(); err != nil {
		return err
	}

	t.node = nil

	return nil
}

func (t *TestCatsServer) Start() error {
	networkMagic := GetNetworkMagic(t.config.NetworkType)
	args := []string{
		"generate-config",
		"--provider-name", "gouroboros",
		"--provider-network-magic", fmt.Sprint(networkMagic),
		"--provider-socket-path", t.config.SocketPath,
		"--provider-keep-alive", "true",
		"--api-keys", TestCatsServerAPIKey,
		"--api-port", fmt.Sprint(t.config.Port),
		"--output-dir", t.config.ConfigPath,
		"--logs-path", t.config.LogsPath,
	}
	argsRun := []string{"run", "--config", filepath.Join(t.config.ConfigPath, "config.json")}
	binary := ResolvEthernalCatsBinary(t.config.NetworkType)

	if err := RunCommand(binary, args, t.config.StdOut); err != nil {
		return err
	}

	node, err := framework.NewNode(binary, argsRun, t.config.StdOut)
	if err != nil {
		return err
	}

	t.node = node
	t.node.SetShouldForceStop(true)

	return nil
}

func (t TestCatsServer) SocketPath() string {
	return t.config.SocketPath
}

func (t TestCatsServer) Port() int {
	return t.config.Port
}

func (t TestCatsServer) URL() string {
	return fmt.Sprintf("localhost:%d", t.config.Port)
}

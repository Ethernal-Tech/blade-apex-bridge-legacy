package cardanofw

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

type ApexSystem struct {
	PrimeCluster  *TestCardanoCluster
	VectorCluster *TestCardanoCluster
	Nexus         *TestEVMBridge
	BridgeCluster *framework.TestCluster
	Config        *ApexSystemConfig

	validators  []*TestApexValidator
	relayerNode *framework.Node

	PrimeMultisigAddr     string
	PrimeMultisigFeeAddr  string
	VectorMultisigAddr    string
	VectorMultisigFeeAddr string

	pvk  string
	psk  string
	pfvk string
	pfsk string

	vvk  string
	vsk  string
	vfvk string
	vfsk string

	nexusRelayerWallet *crypto.ECDSAKey
	bladeAdmin         *crypto.ECDSAKey

	dataDirPath string
}

func NewApexSystem(
	dataDirPath string, opts ...ApexSystemOptions,
) *ApexSystem {
	config := getDefaultApexSystemConfig()
	for _, opt := range opts {
		opt(config)
	}

	return &ApexSystem{
		Config:      config,
		dataDirPath: dataDirPath,
	}
}

func NewApexCentralizedSystem(
	dataDirPath string, opts ...ApexSystemOptions,
) *ApexSystem {
	config := getDefaultApexCentralizedSystemConfig()
	for _, opt := range opts {
		opt(config)
	}

	return &ApexSystem{
		Config:      config,
		dataDirPath: dataDirPath,
	}
}

func (a *ApexSystem) StopChains() {
	wg := sync.WaitGroup{}
	errorsContainer := [2]error{}

	fmt.Println("Stopping chains...")

	if a.PrimeCluster != nil {
		wg.Add(1)

		go func() {
			defer wg.Done()

			errorsContainer[0] = a.PrimeCluster.Stop()
		}()
	}

	if a.VectorCluster != nil {
		wg.Add(1)

		go func() {
			defer wg.Done()

			errorsContainer[1] = a.VectorCluster.Stop()
		}()
	}

	if a.Nexus != nil {
		wg.Add(1)

		go func() {
			defer wg.Done()

			a.Nexus.Cluster.Stop()
		}()
	}

	if a.BridgeCluster != nil {
		wg.Add(1)

		go func() {
			defer wg.Done()

			fmt.Printf("Cleaning up apex bridge\n")
			a.BridgeCluster.Stop()
			fmt.Printf("Done cleaning up apex bridge\n")
		}()
	}

	wg.Wait()

	fmt.Printf("Chains has been stopped...%v\n", errors.Join(errorsContainer[:]...))
}

func (a *ApexSystem) StartChains(t *testing.T) {
	t.Helper()

	wg := &sync.WaitGroup{}
	errorsContainer := [3]error{}

	wg.Add(a.Config.ServiceCount())

	go func() {
		defer wg.Done()

		a.PrimeCluster, errorsContainer[0] = RunCardanoCluster(t, a.Config.PrimeClusterConfig)
	}()

	if a.Config.VectorEnabled {
		go func() {
			defer wg.Done()

			a.VectorCluster, errorsContainer[1] = RunCardanoCluster(t, a.Config.VectorClusterConfig)
		}()
	}

	if a.Config.NexusEnabled {
		go func() {
			defer wg.Done()

			a.Nexus, errorsContainer[2] = RunEVMChain(t, a.Config)
		}()
	}

	wg.Wait()

	t.Cleanup(a.StopChains) // cleanup everything

	require.NoError(t, errors.Join(errorsContainer[:]...))
}

func (a *ApexSystem) StartBridgeChain(t *testing.T) {
	t.Helper()

	bladeAdmin, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	a.bladeAdmin = bladeAdmin
	a.BridgeCluster = framework.NewTestCluster(t, a.Config.BladeValidatorCount,
		framework.WithBladeAdmin(bladeAdmin.Address().String()),
	)

	// create validators
	a.validators = make([]*TestApexValidator, a.Config.BladeValidatorCount)

	for idx := range a.validators {
		a.validators[idx] = NewTestApexValidator(
			a.dataDirPath, idx+1, a.BridgeCluster, a.BridgeCluster.Servers[idx])
	}

	a.BridgeCluster.WaitForReady(t)
}

func (a *ApexSystem) StartApexCentralized(t *testing.T) {
	t.Helper()

	bladeAdmin, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	a.bladeAdmin = bladeAdmin

	// create validator
	a.validators = make([]*TestApexValidator, 1)

	a.validators[0] = &TestApexValidator{
		dataDirPath: filepath.Join(a.dataDirPath, "validator_1"),
		ID:          1,
		cluster:     nil,
		server:      nil,
	}
}

func (a *ApexSystem) GetPrimeGenesisWallet(t *testing.T) *cardanowallet.Wallet {
	t.Helper()

	result, err := GetGenesisWalletFromCluster(a.PrimeCluster.Config.TmpDir, 2)
	require.NoError(t, err)

	return result
}

func (a *ApexSystem) GetVectorGenesisWallet(t *testing.T) *cardanowallet.Wallet {
	t.Helper()

	result, err := GetGenesisWalletFromCluster(a.VectorCluster.Config.TmpDir, 2)
	require.NoError(t, err)

	return result
}

func (a *ApexSystem) GetPrimeTxProvider() cardanowallet.ITxProvider {
	return cardanowallet.NewTxProviderOgmios(a.PrimeCluster.OgmiosURL())
}

func (a *ApexSystem) GetVectorTxProvider() cardanowallet.ITxProvider {
	return cardanowallet.NewTxProviderOgmios(a.VectorCluster.OgmiosURL())
}

func (a *ApexSystem) CreateAndFundUser(t *testing.T, ctx context.Context, sendAmount uint64) *TestApexUser {
	t.Helper()

	user := NewTestApexUser(t, a.PrimeCluster.Config.NetworkType, a.GetVectorNetworkType())

	txProviderPrime := a.GetPrimeTxProvider()
	// Fund prime address
	primeGenesisWallet := a.GetPrimeGenesisWallet(t)

	user.SendToUser(
		t, ctx, txProviderPrime, primeGenesisWallet, sendAmount, a.PrimeCluster.NetworkConfig())

	fmt.Printf("Prime user address funded\n")

	if a.Config.VectorEnabled {
		txProviderVector := a.GetVectorTxProvider()
		// Fund vector address
		vectorGenesisWallet := a.GetVectorGenesisWallet(t)

		user.SendToUser(
			t, ctx, txProviderVector, vectorGenesisWallet, sendAmount, a.VectorCluster.NetworkConfig())

		fmt.Printf("Vector user address funded\n")
	}

	return user
}

func (a *ApexSystem) CreateAndFundExistingUser(
	t *testing.T, ctx context.Context, primePrivateKey, vectorPrivateKey string, sendAmount uint64,
	primeNetworkConfig TestCardanoNetworkConfig, vectorNetworkConfig TestCardanoNetworkConfig,
) *TestApexUser {
	t.Helper()

	user := NewTestApexUserWithExistingWallets(t, primePrivateKey, vectorPrivateKey,
		primeNetworkConfig.NetworkType, vectorNetworkConfig.NetworkType)

	txProviderPrime := a.GetPrimeTxProvider()
	txProviderVector := a.GetVectorTxProvider()

	// Fund prime address
	primeGenesisWallet := a.GetPrimeGenesisWallet(t)

	user.SendToUser(t, ctx, txProviderPrime, primeGenesisWallet, sendAmount, primeNetworkConfig)

	fmt.Printf("Prime user address funded\n")

	// Fund vector address
	vectorGenesisWallet := a.GetVectorGenesisWallet(t)

	user.SendToUser(t, ctx, txProviderVector, vectorGenesisWallet, sendAmount, vectorNetworkConfig)

	fmt.Printf("Vector user address funded\n")

	return user
}

func (a *ApexSystem) CreateAndFundNexusUser(ctx context.Context, ethAmount uint64) (*wallet.Account, error) {
	user, err := wallet.GenerateAccount()
	if err != nil {
		return nil, err
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(a.Nexus.Cluster.Servers[0].JSONRPC()))
	if err != nil {
		return nil, err
	}

	addr := user.Address()

	receipt, err := txRelayer.SendTransaction(
		types.NewTx(types.NewLegacyTx(
			types.WithFrom(a.Nexus.Admin.Ecdsa.Address()),
			types.WithTo(&addr),
			types.WithValue(ethgo.Ether(ethAmount)),
		)),
		a.Nexus.Admin.Ecdsa)
	if err != nil {
		return nil, err
	} else if receipt.Status != uint64(types.ReceiptSuccess) {
		return nil, fmt.Errorf("fund user tx failed: %d", receipt.Status)
	}

	return user, nil
}

func (a *ApexSystem) GetVectorNetworkType() cardanowallet.CardanoNetworkType {
	if a.Config.VectorEnabled && a.VectorCluster != nil {
		return a.VectorCluster.Config.NetworkType
	}

	return cardanowallet.TestNetNetwork // does not matter really
}

func (a *ApexSystem) GetBridgeAdmin() *crypto.ECDSAKey {
	return a.bladeAdmin
}

func (a *ApexSystem) CreateWallets(createBLSKeys bool) (err error) {
	for _, validator := range a.validators {
		if err = validator.CardanoWalletCreate(ChainIDPrime); err != nil {
			return err
		}

		if a.Config.VectorEnabled {
			if err = validator.CardanoWalletCreate(ChainIDVector); err != nil {
				return err
			}
		}

		if a.Config.NexusEnabled {
			if createBLSKeys {
				if err = validator.createSpecificWallet("batcher-evm"); err != nil {
					return err
				}
			}

			validator.BatcherBN256PrivateKey, err = validator.getBatcherWallet(!createBLSKeys)
			if err != nil {
				return err
			}

			if validator.ID == RunRelayerOnValidatorID {
				if err = validator.createSpecificWallet("relayer-evm"); err != nil {
					return err
				}

				a.nexusRelayerWallet, err = validator.getRelayerWallet()
				if err != nil {
					return err
				}
			}
		}
	}

	return err
}

func (a *ApexSystem) GetNexusRelayerWalletAddr() types.Address {
	return a.nexusRelayerWallet.Address()
}

func (a *ApexSystem) GetValidatorsCount() int {
	return len(a.validators)
}

func (a *ApexSystem) GetValidator(t *testing.T, idx int) *TestApexValidator {
	t.Helper()

	require.True(t, idx >= 0 && idx < len(a.validators))

	return a.validators[idx]
}

func (a *ApexSystem) RegisterChains(fundTokenAmount uint64) error {
	tokenSupply := new(big.Int).SetUint64(fundTokenAmount)

	errs := make([]error, len(a.validators))
	wg := sync.WaitGroup{}

	wg.Add(len(a.validators))

	for i, validator := range a.validators {
		go func(validator *TestApexValidator, indx int) {
			defer wg.Done()

			errs[indx] = validator.RegisterChain(ChainIDPrime, tokenSupply, ChainTypeCardano)
			if errs[indx] != nil {
				return
			}

			if a.Config.VectorEnabled {
				errs[indx] = validator.RegisterChain(ChainIDVector, tokenSupply, ChainTypeCardano)
				if errs[indx] != nil {
					return
				}
			}

			if a.Config.NexusEnabled {
				errs[indx] = validator.RegisterChain(ChainIDNexus, tokenSupply, ChainTypeEVM)
				if errs[indx] != nil {
					return
				}
			}
		}(validator, i)
	}

	wg.Wait()

	return errors.Join(errs...)
}

func (a *ApexSystem) GenerateConfigs(
	primeCluster *TestCardanoCluster,
	vectorCluster *TestCardanoCluster,
	nexus *TestEVMBridge,
) error {
	errs := make([]error, len(a.validators))
	wg := sync.WaitGroup{}

	wg.Add(len(a.validators))

	for i, validator := range a.validators {
		go func(validator *TestApexValidator, indx int) {
			defer wg.Done()

			telemetryConfig := ""
			if indx == 0 {
				telemetryConfig = a.Config.TelemetryConfig
			}

			var (
				primeNetworkAddr   string
				primeNetworkMagic  uint
				primeNetworkID     uint
				vectorOgmiosURL    = "http://localhost:1000"
				vectorNetworkAddr  = "localhost:5499"
				vectorNetworkMagic = uint(0)
				vectorNetworkID    = uint(0)
				nexusNodeURL       = "http://localhost:5500"
			)

			if a.Config.TargetOneCardanoClusterServer {
				primeNetworkAddr = primeCluster.NetworkAddress()
				primeNetworkMagic = primeCluster.Config.NetworkMagic
				primeNetworkID = uint(primeCluster.Config.NetworkType)
			} else {
				primeServer := primeCluster.Servers[indx%len(primeCluster.Servers)]
				primeNetworkAddr = primeServer.NetworkAddress()
				primeNetworkMagic = primeServer.config.NetworkMagic
				primeNetworkID = uint(primeServer.config.NetworkID)
			}

			if a.Config.VectorEnabled {
				vectorOgmiosURL = vectorCluster.OgmiosURL()

				if a.Config.TargetOneCardanoClusterServer {
					vectorNetworkAddr = vectorCluster.NetworkAddress()
					vectorNetworkMagic = vectorCluster.Config.NetworkMagic
					vectorNetworkID = uint(vectorCluster.Config.NetworkType)
				} else {
					vectorServer := vectorCluster.Servers[indx%len(vectorCluster.Servers)]
					vectorNetworkAddr = vectorServer.NetworkAddress()
					vectorNetworkMagic = vectorServer.config.NetworkMagic
					vectorNetworkID = uint(vectorServer.config.NetworkID)
				}
			}

			if a.Config.NexusEnabled {
				nexusNodeURLIndx := 0
				if a.Config.TargetOneCardanoClusterServer {
					nexusNodeURLIndx = indx % len(nexus.Cluster.Servers)
				}

				nexusNodeURL = nexus.Cluster.Servers[nexusNodeURLIndx].JSONRPCAddr()
			}

			errs[indx] = validator.GenerateConfigs(
				primeNetworkAddr,
				primeNetworkMagic,
				primeNetworkID,
				primeCluster.OgmiosURL(),
				a.Config.PrimeSlotRoundingThreshold,
				a.Config.PrimeTTLInc,
				vectorNetworkAddr,
				vectorNetworkMagic,
				vectorNetworkID,
				vectorOgmiosURL,
				a.Config.VectorSlotRoundingThreshold,
				a.Config.VectorTTLInc,
				a.Config.APIPortStart+indx,
				a.Config.APIKey,
				telemetryConfig,
				nexusNodeURL,
				a.PrimeMultisigAddr,
				a.PrimeMultisigFeeAddr,
				a.VectorMultisigAddr,
				a.VectorMultisigFeeAddr,
				a.pvk,
				a.psk,
				a.pfvk,
				a.pfsk,
				a.vvk,
				a.vsk,
				a.vfvk,
				a.vfsk,
			)
		}(validator, i)
	}

	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		return err
	}

	if handler := a.Config.CustomOracleHandler; handler != nil {
		for _, val := range a.validators {
			err := UpdateJSONFile(
				val.GetValidatorComponentsConfig(), val.GetValidatorComponentsConfig(), handler, false)
			if err != nil {
				return err
			}
		}
	}

	if handler := a.Config.CustomRelayerHandler; handler != nil {
		for _, val := range a.validators {
			if RunRelayerOnValidatorID != val.ID {
				continue
			}

			if err := UpdateJSONFile(val.GetRelayerConfig(), val.GetRelayerConfig(), handler, false); err != nil {
				return err
			}
		}
	}

	return nil
}

func (a *ApexSystem) StartValidatorComponents(ctx context.Context) (err error) {
	for _, validator := range a.validators {
		hasAPI := a.Config.APIValidatorID == -1 || validator.ID == a.Config.APIValidatorID

		if err = validator.Start(ctx, hasAPI); err != nil {
			return err
		}
	}

	return err
}

func (a *ApexSystem) StartRelayer(ctx context.Context) (err error) {
	for _, validator := range a.validators {
		if RunRelayerOnValidatorID != validator.ID {
			continue
		}

		a.relayerNode, err = framework.NewNodeWithContext(ctx, ResolveApexBridgeBinary(), []string{
			"run-relayer",
			"--config", validator.GetRelayerConfig(),
		}, os.Stdout)
		if err != nil {
			return err
		}
	}

	return nil
}

func (a ApexSystem) StopRelayer() error {
	if a.relayerNode == nil {
		return errors.New("relayer not started")
	}

	return a.relayerNode.Stop()
}

func (a *ApexSystem) GetBridgingAPI() (string, error) {
	apis, err := a.GetBridgingAPIs()
	if err != nil {
		return "", err
	}

	return apis[0], nil
}

func (a *ApexSystem) GetBridgingAPIs() (res []string, err error) {
	for _, validator := range a.validators {
		hasAPI := a.Config.APIValidatorID == -1 || validator.ID == a.Config.APIValidatorID

		if hasAPI {
			if validator.APIPort == 0 {
				return nil, fmt.Errorf("api port not defined")
			}

			res = append(res, fmt.Sprintf("http://localhost:%d", validator.APIPort))
		}
	}

	if len(res) == 0 {
		return nil, fmt.Errorf("not running API")
	}

	return res, nil
}

func (a *ApexSystem) ApexBridgeProcessesRunning() bool {
	if a.relayerNode == nil || a.relayerNode.ExitResult() != nil {
		return false
	}

	for _, validator := range a.validators {
		if validator.node == nil || validator.node.ExitResult() != nil {
			return false
		}
	}

	return true
}

func (a *ApexSystem) FundCardanoMultisigAddresses(
	ctx context.Context, fundTokenAmount uint64,
) error {
	const (
		numOfRetries = 90
		waitTime     = time.Second * 2
	)

	fund := func(cluster *TestCardanoCluster, fundTokenAmount uint64, addr string) (string, error) {
		txProvider := cardanowallet.NewTxProviderOgmios(cluster.OgmiosURL())

		defer txProvider.Dispose()

		genesisWallet, err := GetGenesisWalletFromCluster(cluster.Config.TmpDir, 1)
		if err != nil {
			return "", err
		}

		txHash, err := SendTx(ctx, txProvider, genesisWallet, fundTokenAmount,
			addr, cluster.NetworkConfig(), []byte{})
		if err != nil {
			return "", err
		}

		err = cardanowallet.WaitForTxHashInUtxos(
			ctx, txProvider, addr, txHash, numOfRetries, waitTime, IsRecoverableError)
		if err != nil {
			return "", err
		}

		return txHash, nil
	}

	txHash, err := fund(a.PrimeCluster, fundTokenAmount, a.PrimeMultisigAddr)
	if err != nil {
		return err
	}

	fmt.Printf("Prime multisig addr funded: %s\n", txHash)

	txHash, err = fund(a.PrimeCluster, fundTokenAmount, a.PrimeMultisigFeeAddr)
	if err != nil {
		return err
	}

	fmt.Printf("Prime fee addr funded: %s\n", txHash)

	if a.Config.VectorEnabled {
		fmt.Printf("DN_LOG_TAG addr addr: %s\n", a.VectorMultisigAddr)
		txHash, err := fund(a.VectorCluster, fundTokenAmount, a.VectorMultisigAddr)
		if err != nil {
			return err
		}

		fmt.Printf("Vector multisig addr funded: %s\n", txHash)

		txHash, err = fund(a.VectorCluster, fundTokenAmount, a.VectorMultisigFeeAddr)
		if err != nil {
			return err
		}

		fmt.Printf("Vector fee addr funded: %s\n", txHash)
	}

	return nil
}

func (a *ApexSystem) CreateCardanoMultisigAddresses() (err error) {
	a.PrimeMultisigAddr, a.PrimeMultisigFeeAddr, err = a.cardanoCreateAddress(a.PrimeCluster.Config.NetworkType, nil)
	if err != nil {
		return fmt.Errorf("failed to create addresses for prime: %w", err)
	}

	if a.Config.VectorEnabled {
		a.VectorMultisigAddr, a.VectorMultisigFeeAddr, err = a.cardanoCreateAddress(a.VectorCluster.Config.NetworkType, nil)
		if err != nil {
			fmt.Printf("DN_LOG_TAG error: failed to create addresses for vector: %+v\n", err)
			return fmt.Errorf("failed to create addresses for vector: %w", err)
		}
		fmt.Printf("DN_LOG_TAG addr addr: %s %s\n", a.VectorMultisigAddr, a.VectorMultisigFeeAddr)
	}

	return nil
}

func (a *ApexSystem) CreateCardanoAddresses() (err error) {
	fmt.Println("CreateCardanoAddresses")

	cw, err := cardanowallet.GenerateWallet(false)
	if err != nil {
		return fmt.Errorf("failed to create prime wallet: %w", err)
	}

	a.PrimeMultisigAddr, _, err = a.cardanoCreateAddress(a.PrimeCluster.Config.NetworkType,
		[]string{hex.EncodeToString(cw.VerificationKey), hex.EncodeToString(cw.SigningKey)})
	if err != nil {
		return fmt.Errorf("failed to create prime address: %w", err)
	}

	a.pvk = hex.EncodeToString(cw.VerificationKey)
	a.psk = hex.EncodeToString(cw.SigningKey)

	cfw, err := cardanowallet.GenerateWallet(false)
	if err != nil {
		return fmt.Errorf("failed to create prime wallet: %w", err)
	}

	a.PrimeMultisigFeeAddr, _, err = a.cardanoCreateAddress(a.PrimeCluster.Config.NetworkType,
		[]string{hex.EncodeToString(cfw.VerificationKey), hex.EncodeToString(cfw.SigningKey)})
	if err != nil {
		return fmt.Errorf("failed to create prime fee address: %w", err)
	}

	a.pfsk = hex.EncodeToString(cfw.SigningKey)
	a.pfvk = hex.EncodeToString(cfw.VerificationKey)

	vw, err := cardanowallet.GenerateWallet(false)
	if err != nil {
		return fmt.Errorf("failed to create vector wallet: %w", err)
	}

	fmt.Printf("DN_LOG_TAG a.VectorCluster.Config.NetworkType %+v\n", a.VectorCluster.Config.NetworkType)
	// a.VectorMultisigAddr, _, err = a.cardanoCreateAddress(a.VectorCluster.Config.NetworkType,
	// 	[]string{hex.EncodeToString(vw.VerificationKey), hex.EncodeToString(vw.SigningKey)})
	// if err != nil {
	// 	return fmt.Errorf("failed to create vector address: %w", err)
	// }

	addrVec, err := cardanowallet.NewEnterpriseAddress(a.VectorCluster.Config.NetworkType,
		vw.VerificationKey)
	if err != nil {
		return fmt.Errorf("failed to create vector fee address: %w", err)
	}

	a.VectorMultisigAddr = addrVec.String()

	a.vsk = hex.EncodeToString(vw.SigningKey)
	a.vvk = hex.EncodeToString(vw.VerificationKey)

	vfw, err := cardanowallet.GenerateWallet(false)
	if err != nil {
		return fmt.Errorf("failed to create vector wallet: %w", err)
	}

	// a.VectorMultisigFeeAddr, _, err = a.cardanoCreateAddress(a.VectorCluster.Config.NetworkType,
	// 	[]string{hex.EncodeToString(vfw.VerificationKey), hex.EncodeToString(vfw.SigningKey)})

	addrFVec, err := cardanowallet.NewEnterpriseAddress(a.VectorCluster.Config.NetworkType,
		vfw.VerificationKey)
	if err != nil {
		return fmt.Errorf("failed to create vector fee address: %w", err)
	}

	a.VectorMultisigFeeAddr = addrFVec.String()

	a.vfsk = hex.EncodeToString(vfw.SigningKey)
	a.vfvk = hex.EncodeToString(vfw.VerificationKey)

	fmt.Printf("DN_LOG_TAG addr addr: %s %s\n", a.VectorMultisigAddr, a.VectorMultisigFeeAddr)

	return nil
}

func (a *ApexSystem) cardanoCreateAddress(
	network cardanowallet.CardanoNetworkType, keys []string,
) (string, string, error) {
	bridgeAdminPk, err := a.bladeAdmin.MarshallPrivateKey()
	if err != nil {
		return "", "", err
	}

	bothAddresses := false

	args := []string{
		"create-address",
		"--network-id", fmt.Sprint(network),
	}

	if len(keys) == 0 {
		args = append(args,
			"--bridge-url", a.BridgeCluster.Servers[0].JSONRPCAddr(),
			"--bridge-addr", contracts.Bridge.String(),
			"--chain", GetNetworkName(network),
			"--bridge-key", hex.EncodeToString(bridgeAdminPk),
			"--multisig", "false")

		bothAddresses = true
	}

	for _, key := range keys {
		args = append(args, "--key", key)
	}

	args = append(args, "--multisig", "false")

	var outb bytes.Buffer

	err = RunCommand(ResolveApexBridgeBinary(), args, io.MultiWriter(os.Stdout, &outb))
	if err != nil {
		return "", "", err
	}

	if !bothAddresses {
		return strings.TrimSpace(strings.ReplaceAll(outb.String(), "Address = ", "")), "", nil
	}

	multisig, fee, output := "", "", outb.String()
	reGateway := regexp.MustCompile(`Multisig Address\s*=\s*([^\s]+)`)
	reNativeTokenWallet := regexp.MustCompile(`Fee Payer Address\s*=\s*([^\s]+)`)

	if match := reGateway.FindStringSubmatch(output); len(match) > 0 {
		multisig = match[1]
	}

	if match := reNativeTokenWallet.FindStringSubmatch(output); len(match) > 0 {
		fee = match[1]
	}

	return multisig, fee, nil
}

func (a *ApexSystem) GetPrimeMSigVerificationKey() string {
	return a.pvk
}

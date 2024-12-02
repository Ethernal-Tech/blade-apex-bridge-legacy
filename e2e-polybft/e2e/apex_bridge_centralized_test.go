package e2e

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

//nolint:dupl
func TestE2E_ApexBridgeCentralized_InvalidScenarios(t *testing.T) {
	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.SetupAndRunApexCentralizedBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.CreateAndFundUser(t, ctx, uint64(50_000_000))

	txProviderPrime := apex.GetPrimeTxProvider()

	t.Run("Submitter not enough funds", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)

		receivers := map[string]uint64{
			user.VectorAddress: sendAmount * 10, // 10Ada
		}

		bridgingRequestMetadata, err := cardanofw.CreateMetaData(
			user.PrimeAddress, receivers, cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()), feeAmount)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTx(
			ctx, txProviderPrime, user.PrimeWallet, sendAmount+feeAmount, apex.PrimeMultisigAddr,
			apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
		require.NoError(t, err)

		apiURL, err := apex.GetBridgingAPI()
		require.NoError(t, err)
		cardanofw.WaitForInvalidState(t, ctx, apiURL, apiKey, "prime", txHash)
	})

	t.Run("Multiple submitters don't have enough funds", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			sendAmount := uint64(1_000_000)
			feeAmount := uint64(1_100_000)

			receivers := map[string]uint64{
				user.VectorAddress: sendAmount * 10, // 10Ada
			}

			bridgingRequestMetadata, err := cardanofw.CreateMetaData(
				user.PrimeAddress, receivers, cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()), feeAmount)
			require.NoError(t, err)

			txHash, err := cardanofw.SendTx(
				ctx, txProviderPrime, user.PrimeWallet, sendAmount+feeAmount, apex.PrimeMultisigAddr,
				apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
			require.NoError(t, err)

			apiURL, err := apex.GetBridgingAPI()
			require.NoError(t, err)
			cardanofw.WaitForInvalidState(t, ctx, apiURL, apiKey, "prime", txHash)
		}
	})
	/*
		t.Run("Multiple submitters don't have enough funds parallel", func(t *testing.T) {
			var err error

			instances := 5
			walletKeys := make([]wallet.IWallet, instances)
			txHashes := make([]string, instances)
			primeGenesisWallet := apex.GetPrimeGenesisWallet(t)

			for i := 0; i < instances; i++ {
				walletKeys[i], err = wallet.GenerateWallet(false)
				require.NoError(t, err)

				walletAddress, err := cardanofw.GetAddress(apex.PrimeCluster.NetworkConfig().NetworkType, walletKeys[i])
				require.NoError(t, err)

				fundSendAmount := uint64(5_000_000)
				txHash, err := cardanofw.SendTx(ctx, txProviderPrime, primeGenesisWallet,
					fundSendAmount, walletAddress.String(), apex.PrimeCluster.NetworkConfig(), []byte{})
				require.NoError(t, err)
				err = wallet.WaitForTxHashInUtxos(ctx, txProviderPrime, walletAddress.String(), txHash,
					60, time.Second*2, cardanofw.IsRecoverableError)
				require.NoError(t, err)
			}

			sendAmount := uint64(1_000_000)
			feeAmount := uint64(1_100_000)

			var wg sync.WaitGroup

			for i := 0; i < instances; i++ {
				idx := i
				receivers := map[string]uint64{
					user.VectorAddress: sendAmount * 10, // 10Ada
				}

				bridgingRequestMetadata, err := cardanofw.CreateMetaData(
					user.PrimeAddress, receivers, cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()), feeAmount)
				require.NoError(t, err)

				wg.Add(1)

				go func() {
					defer wg.Done()

					txHashes[idx], err = cardanofw.SendTx(
						ctx, txProviderPrime, walletKeys[idx], sendAmount+feeAmount, apex.PrimeMultisigAddr,
						apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
					require.NoError(t, err)
				}()
			}

			wg.Wait()

			for i := 0; i < instances; i++ {
				apiURL, err := apex.GetBridgingAPI()
				require.NoError(t, err)
				cardanofw.WaitForInvalidState(t, ctx, apiURL, apiKey, "prime", txHashes[i])
			}
		})
	*/
	t.Run("Submitted invalid metadata", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)

		receivers := map[string]uint64{
			user.VectorAddress: sendAmount,
		}

		bridgingRequestMetadata, err := cardanofw.CreateMetaData(
			user.PrimeAddress, receivers, cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()), feeAmount)
		require.NoError(t, err)

		// Send only half bytes of metadata making it invalid
		bridgingRequestMetadata = bridgingRequestMetadata[0 : len(bridgingRequestMetadata)/2]

		_, err = cardanofw.SendTx(
			ctx, txProviderPrime, user.PrimeWallet, sendAmount+feeAmount, apex.PrimeMultisigAddr,
			apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
		require.Error(t, err)
	})

	t.Run("Submitted invalid metadata - wrong type", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)

		receivers := map[string]uint64{
			user.VectorAddress:         sendAmount,
			apex.VectorMultisigFeeAddr: feeAmount,
		}

		var transactions = make([]cardanofw.BridgingRequestMetadataTransaction, 0, len(receivers))
		for addr, amount := range receivers {
			transactions = append(transactions, cardanofw.BridgingRequestMetadataTransaction{
				Address: cardanofw.SplitString(addr, 40),
				Amount:  amount,
			})
		}

		metadata := map[string]interface{}{
			"1": map[string]interface{}{
				"t":  "transaction", // should be "bridge"
				"d":  cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()),
				"s":  cardanofw.SplitString(user.PrimeAddress, 40),
				"tx": transactions,
			},
		}

		bridgingRequestMetadata, err := json.Marshal(metadata)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTx(
			ctx, txProviderPrime, user.PrimeWallet, sendAmount+feeAmount, apex.PrimeMultisigAddr,
			apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
		require.NoError(t, err)

		apiURL, err := apex.GetBridgingAPI()
		require.NoError(t, err)

		requestURL := fmt.Sprintf(
			"%s/api/BridgingRequestState/Get?chainId=%s&txHash=%s", apiURL, "prime", txHash)

		_, err = cardanofw.WaitForRequestStates(nil, ctx, requestURL, apiKey, 60)
		require.Error(t, err)
		require.ErrorContains(t, err, "Timeout")
	})

	t.Run("Submitted invalid metadata - invalid destination", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)

		receivers := map[string]uint64{
			user.VectorAddress:         sendAmount,
			apex.VectorMultisigFeeAddr: feeAmount,
		}

		var transactions = make([]cardanofw.BridgingRequestMetadataTransaction, 0, len(receivers))
		for addr, amount := range receivers {
			transactions = append(transactions, cardanofw.BridgingRequestMetadataTransaction{
				Address: cardanofw.SplitString(addr, 40),
				Amount:  amount,
			})
		}

		metadata := map[string]interface{}{
			"1": map[string]interface{}{
				"t":  "bridge",
				"d":  "", // should be destination chain address
				"s":  cardanofw.SplitString(user.PrimeAddress, 40),
				"tx": transactions,
			},
		}

		bridgingRequestMetadata, err := json.Marshal(metadata)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTx(
			ctx, txProviderPrime, user.PrimeWallet, sendAmount+feeAmount, apex.PrimeMultisigAddr,
			apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
		require.NoError(t, err)

		apiURL, err := apex.GetBridgingAPI()
		require.NoError(t, err)
		cardanofw.WaitForInvalidState(t, ctx, apiURL, apiKey, "prime", txHash)
	})

	t.Run("Submitted invalid metadata - invalid sender", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)

		receivers := map[string]uint64{
			user.VectorAddress:         sendAmount,
			apex.VectorMultisigFeeAddr: feeAmount,
		}

		var transactions = make([]cardanofw.BridgingRequestMetadataTransaction, 0, len(receivers))
		for addr, amount := range receivers {
			transactions = append(transactions, cardanofw.BridgingRequestMetadataTransaction{
				Address: cardanofw.SplitString(addr, 40),
				Amount:  amount,
			})
		}

		metadata := map[string]interface{}{
			"1": map[string]interface{}{
				"t":  "bridge",
				"d":  cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()),
				"s":  "", // should be sender address (max len 40)
				"tx": transactions,
			},
		}

		bridgingRequestMetadata, err := json.Marshal(metadata)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTx(
			ctx, txProviderPrime, user.PrimeWallet, sendAmount+feeAmount, apex.PrimeMultisigAddr,
			apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
		require.NoError(t, err)

		apiURL, err := apex.GetBridgingAPI()
		require.NoError(t, err)
		cardanofw.WaitForInvalidState(t, ctx, apiURL, apiKey, "prime", txHash)
	})

	t.Run("Submitted invalid metadata - empty tx", func(t *testing.T) {
		var transactions = make([]cardanofw.BridgingRequestMetadataTransaction, 0)

		metadata := map[string]interface{}{
			"1": map[string]interface{}{
				"t":  "bridge",
				"d":  cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()),
				"s":  cardanofw.SplitString(user.PrimeAddress, 40),
				"tx": transactions, // should not be empty
			},
		}

		bridgingRequestMetadata, err := json.Marshal(metadata)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTx(
			ctx, txProviderPrime, user.PrimeWallet, 1_000_000, apex.PrimeMultisigAddr,
			apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
		require.NoError(t, err)

		apiURL, err := apex.GetBridgingAPI()
		require.NoError(t, err)
		cardanofw.WaitForInvalidState(t, ctx, apiURL, apiKey, "prime", txHash)
	})
}

func TestE2E_ApexBridgeCentralized_ValidScenarios(t *testing.T) {
	const (
		apiKey               = "test_api_key"
		maxParallelInstances = 50
		fundAmount           = uint64(50_000_000_000)
	)

	var (
		err                error
		walletKeysPrime    = make([]*wallet.Wallet, maxParallelInstances)
		walletKeysVector   = make([]*wallet.Wallet, maxParallelInstances)
		primeClusterConfig = &cardanofw.RunCardanoClusterConfig{
			ID:                 0,
			NodesCount:         4,
			NetworkType:        wallet.TestNetNetwork,
			InitialFundsAmount: fundAmount,
			InitialFundsKeys:   make([]string, maxParallelInstances),
		}
		vectorClusterConfig = &cardanofw.RunCardanoClusterConfig{
			ID:                 1,
			NodesCount:         4,
			NetworkType:        wallet.VectorTestNetNetwork,
			InitialFundsAmount: fundAmount,
			InitialFundsKeys:   make([]string, maxParallelInstances),
		}
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	for i := range walletKeysPrime {
		walletKeysPrime[i], err = wallet.GenerateWallet(false)
		require.NoError(t, err)

		walletKeysVector[i], err = wallet.GenerateWallet(false)
		require.NoError(t, err)

		addrPrime, err := wallet.NewEnterpriseAddress(primeClusterConfig.NetworkType, walletKeysPrime[i].VerificationKey)
		require.NoError(t, err)

		addrVec, err := wallet.NewEnterpriseAddress(vectorClusterConfig.NetworkType, walletKeysVector[i].VerificationKey)
		require.NoError(t, err)

		primeClusterConfig.InitialFundsKeys[i] = hex.EncodeToString(addrPrime.Bytes())
		vectorClusterConfig.InitialFundsKeys[i] = hex.EncodeToString(addrVec.Bytes())
	}

	apex := cardanofw.SetupAndRunApexCentralizedBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithPrimeClusterConfig(primeClusterConfig),
		cardanofw.WithVectorClusterConfig(vectorClusterConfig),
	)

	// defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.CreateAndFundUser(t, ctx, uint64(20_000_000_000))

	txProviderPrime := apex.GetPrimeTxProvider()
	txProviderVector := apex.GetVectorTxProvider()

	primeGenesisWallet := apex.GetPrimeGenesisWallet(t)
	vectorGenesisWallet := apex.GetVectorGenesisWallet(t)

	primeGenesisAddress, err := cardanofw.GetAddress(apex.PrimeCluster.NetworkConfig().NetworkType, primeGenesisWallet)
	require.NoError(t, err)

	vectorGenesisAddress, err := cardanofw.GetAddress(apex.VectorCluster.NetworkConfig().NetworkType, vectorGenesisWallet)
	require.NoError(t, err)

	fmt.Println("prime genesis addr: ", primeGenesisAddress)
	fmt.Println("vector genesis addr: ", vectorGenesisAddress)
	fmt.Println("prime user addr: ", user.PrimeAddress)
	fmt.Println("vector user addr: ", user.VectorAddress)
	fmt.Println("prime multisig addr: ", apex.PrimeMultisigAddr)
	fmt.Println("prime fee addr: ", apex.PrimeMultisigFeeAddr)
	fmt.Println("vector multisig addr: ", apex.VectorMultisigAddr)
	fmt.Println("vector fee addr: ", apex.VectorMultisigFeeAddr)

	t.Run("1. From prime to vector one by one - wait for other side", func(t *testing.T) {
		const (
			sendAmount = uint64(1_000_000)
			instances  = 5
		)

		for i := 0; i < instances; i++ {
			prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
			require.NoError(t, err)

			fmt.Printf("%v - prevAmount %v\n", i+1, prevAmount)

			txHash := user.BridgeAmount(t, ctx, txProviderPrime, apex.PrimeMultisigAddr,
				apex.VectorMultisigFeeAddr, sendAmount, apex.PrimeCluster.NetworkConfig())

			fmt.Printf("%v - Tx sent. hash: %s\n", i+1, txHash)

			expectedAmount := prevAmount + sendAmount
			fmt.Printf("%v - expectedAmount %v\n", i+1, expectedAmount)

			err = cardanofw.WaitForAmount(ctx, txProviderVector, user.VectorAddress, func(val uint64) bool {
				return val == expectedAmount
			}, 20, time.Second*10)
			require.NoError(t, err)
		}
	})

	t.Run("2. From prime to vector one by one", func(t *testing.T) {
		const (
			sendAmount = uint64(1_000_000)
			instances  = 5
		)

		prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)

		for i := 0; i < instances; i++ {
			txHash := user.BridgeAmount(t, ctx, txProviderPrime, apex.PrimeMultisigAddr,
				apex.VectorMultisigFeeAddr, sendAmount, apex.PrimeCluster.NetworkConfig())

			fmt.Printf("Tx %v sent. hash: %s\n", i+1, txHash)
		}

		expectedAmount := prevAmount + uint64(instances)*sendAmount
		err = cardanofw.WaitForAmount(ctx, txProviderVector, user.VectorAddress, func(val uint64) bool {
			return val == expectedAmount
		}, 100, time.Second*5)
		require.NoError(t, err)
	})
	t.Run("3. From prime to vector parallel", func(t *testing.T) {
		const (
			sendAmount = uint64(1_000_000)
			instances  = 5
		)

		prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)

		var wg sync.WaitGroup
		for i := 0; i < instances; i++ {
			wg.Add(1)

			go func(idx int) {
				defer wg.Done()

				txHash := cardanofw.BridgeAmountFull(t, ctx, txProviderPrime, apex.PrimeCluster.NetworkConfig(),
					apex.PrimeMultisigAddr, apex.VectorMultisigFeeAddr, walletKeysPrime[idx],
					user.VectorAddress, sendAmount)
				fmt.Printf("Tx %v sent. hash: %s\n", idx+1, txHash)
			}(i)
		}

		wg.Wait()

		expectedAmount := prevAmount + uint64(instances)*sendAmount
		err = cardanofw.WaitForAmount(ctx, txProviderVector, user.VectorAddress, func(val uint64) bool {
			return val == expectedAmount
		}, 100, time.Second*5)
		require.NoError(t, err)
		fmt.Printf("%v TXs confirmed\n", instances)
	})

	t.Run("4. From vector to prime one by one", func(t *testing.T) {
		const (
			sendAmount = uint64(1_000_000)
			instances  = 5
		)

		prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)

		for i := 0; i < instances; i++ {
			txHash := user.BridgeAmount(t, ctx, txProviderVector, apex.VectorMultisigAddr,
				apex.PrimeMultisigFeeAddr, sendAmount, apex.VectorCluster.NetworkConfig())

			fmt.Printf("Tx %v sent. hash: %s\n", i+1, txHash)
		}

		expectedAmount := prevAmount + uint64(instances)*sendAmount
		err = cardanofw.WaitForAmount(ctx, txProviderPrime, user.PrimeAddress, func(val uint64) bool {
			return val == expectedAmount
		}, 100, time.Second*5)
		require.NoError(t, err)
	})

	t.Run("5. From vector to prime parallel", func(t *testing.T) {
		const (
			instances  = 5
			sendAmount = uint64(1_000_000)
		)

		prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)

		var wg sync.WaitGroup
		for i := 0; i < instances; i++ {
			wg.Add(1)

			go func(idx int) {
				defer wg.Done()

				txHash := cardanofw.BridgeAmountFull(t, ctx, txProviderVector, apex.VectorCluster.NetworkConfig(),
					apex.VectorMultisigAddr, apex.PrimeMultisigFeeAddr, walletKeysVector[idx],
					user.PrimeAddress, sendAmount)
				fmt.Printf("Tx %v sent. hash: %s\n", idx+1, txHash)
			}(i)
		}

		wg.Wait()

		expectedAmount := prevAmount + uint64(instances)*sendAmount
		err = cardanofw.WaitForAmount(ctx, txProviderPrime, user.PrimeAddress, func(val uint64) bool {
			return val == expectedAmount
		}, 100, time.Second*5)
		require.NoError(t, err)
		fmt.Printf("%v TXs confirmed\n", instances)
	})
	t.Run("6. From prime to vector sequential and parallel", func(t *testing.T) {
		const (
			sequentialInstances = 5
			parallelInstances   = 10
			sendAmount          = uint64(1_000_000)
		)

		var wg sync.WaitGroup

		prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)

		for j := 0; j < sequentialInstances; j++ {
			for i := 0; i < parallelInstances; i++ {
				wg.Add(1)

				go func(run, idx int) {
					defer wg.Done()

					txHash := cardanofw.BridgeAmountFull(t, ctx, txProviderPrime, apex.PrimeCluster.NetworkConfig(),
						apex.PrimeMultisigAddr, apex.VectorMultisigFeeAddr, walletKeysPrime[idx],
						user.VectorAddress, sendAmount)
					fmt.Printf("run: %v. Tx %v sent. hash: %s\n", run+1, idx+1, txHash)
				}(j, i)
			}

			wg.Wait()
		}

		fmt.Printf("Waiting for %v TXs\n", sequentialInstances*parallelInstances)

		expectedAmount := prevAmount + uint64(sequentialInstances)*uint64(parallelInstances)*sendAmount
		err = cardanofw.WaitForAmount(ctx, txProviderVector, user.VectorAddress, func(val uint64) bool {
			return val == expectedAmount
		}, 100, time.Second*10)
		require.NoError(t, err)
		fmt.Printf("%v TXs confirmed\n", sequentialInstances*parallelInstances)
	})
	t.Run("7. From prime to vector sequential and parallel with max receivers", func(t *testing.T) {
		const (
			sequentialInstances = 5
			parallelInstances   = 10
			receivers           = 4
			sendAmount          = uint64(1_000_000)
		)

		var (
			wg                           sync.WaitGroup
			destinationWalletKeys        = make([]*wallet.Wallet, receivers)
			destinationWalletAddresses   = make([]string, receivers)
			destinationWalletPrevAmounts = make([]uint64, receivers)
		)

		for i := 0; i < receivers; i++ {
			destinationWalletKeys[i], err = wallet.GenerateWallet(false)
			require.NoError(t, err)

			walletAddress, err := cardanofw.GetAddress(apex.VectorCluster.NetworkConfig().NetworkType, destinationWalletKeys[i])
			require.NoError(t, err)

			destinationWalletAddresses[i] = walletAddress.String()

			destinationWalletPrevAmounts[i], err = cardanofw.GetTokenAmount(ctx, txProviderVector, destinationWalletAddresses[i])
			require.NoError(t, err)
		}

		for j := 0; j < sequentialInstances; j++ {
			for i := 0; i < parallelInstances; i++ {
				wg.Add(1)

				go func(run, idx int) {
					defer wg.Done()

					txHash := cardanofw.BridgeAmountFullMultipleReceivers(t, ctx, txProviderPrime, apex.PrimeCluster.NetworkConfig(),
						apex.PrimeMultisigAddr, apex.VectorMultisigFeeAddr, walletKeysPrime[idx],
						destinationWalletAddresses, sendAmount)
					fmt.Printf("run: %v. Tx %v sent. hash: %s\n", run+1, idx+1, txHash)
				}(j, i)
			}

			wg.Wait()
		}

		fmt.Printf("Waiting for %v TXs\n", sequentialInstances*parallelInstances)

		var wgResult sync.WaitGroup

		for i := 0; i < receivers; i++ {
			wgResult.Add(1)

			go func(receiverIdx int) {
				defer wgResult.Done()

				expectedAmount := destinationWalletPrevAmounts[receiverIdx] + uint64(sequentialInstances)*uint64(parallelInstances)*sendAmount
				err = cardanofw.WaitForAmount(ctx, txProviderVector, destinationWalletAddresses[receiverIdx], func(val uint64) bool {
					return val == expectedAmount
				}, 100, time.Second*10)
				require.NoError(t, err)
				fmt.Printf("%v receiver, %v TXs confirmed\n", receiverIdx, sequentialInstances*parallelInstances)
			}(i)
		}

		wgResult.Wait()
	})
	t.Run("8. Both directions sequential", func(t *testing.T) {
		const (
			instances  = 5
			sendAmount = uint64(1_000_000)
		)

		primeSender := apex.CreateAndFundUser(t, ctx, uint64(20_000_000_000))
		vectorSender := apex.CreateAndFundUser(t, ctx, uint64(20_000_000_000))

		prevAmountOnVector, err := cardanofw.GetTokenAmount(ctx, txProviderVector, primeSender.VectorAddress)
		require.NoError(t, err)
		prevAmountOnPrime, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, vectorSender.PrimeAddress)
		require.NoError(t, err)

		for i := 0; i < instances; i++ {
			primeTxHash := primeSender.BridgeAmount(t, ctx, txProviderPrime, apex.PrimeMultisigAddr,
				apex.VectorMultisigFeeAddr, sendAmount, apex.PrimeCluster.NetworkConfig())

			fmt.Printf("prime tx %v sent. hash: %s\n", i+1, primeTxHash)

			vectorTxHash := vectorSender.BridgeAmount(t, ctx, txProviderVector, apex.VectorMultisigAddr,
				apex.PrimeMultisigFeeAddr, sendAmount, apex.VectorCluster.NetworkConfig())

			fmt.Printf("vector tx %v sent. hash: %s\n", i+1, vectorTxHash)
		}

		fmt.Printf("Waiting for %v TXs on vector\n", instances)
		expectedAmountOnVector := prevAmountOnVector + uint64(instances)*sendAmount
		err = cardanofw.WaitForAmount(ctx, txProviderVector, primeSender.VectorAddress, func(val uint64) bool {
			return val >= expectedAmountOnVector
		}, 200, time.Second*2)
		// require.NoError(t, err)
		if err != nil {
			fmt.Printf("ERROR TX ON VECTOR")
		}

		fmt.Printf("Waiting for %v TXs on prime\n", instances)
		expectedAmountOnPrime := prevAmountOnPrime + uint64(instances)*sendAmount
		err = cardanofw.WaitForAmount(ctx, txProviderPrime, vectorSender.PrimeAddress, func(val uint64) bool {
			return val >= expectedAmountOnPrime
		}, 100, time.Second*2)

		newAmountOnVector, _ := cardanofw.GetTokenAmount(ctx, txProviderVector, primeSender.VectorAddress)
		newAmountOnPrime, _ := cardanofw.GetTokenAmount(ctx, txProviderPrime, vectorSender.PrimeAddress)

		fmt.Printf("prevAmountOnVector: %+v, prevAmountOnPrime: %+v\n", prevAmountOnVector, prevAmountOnPrime)

		fmt.Printf("newAmountOnVector:  %+v, newAmountOnPrime:  %+v\n", newAmountOnVector, newAmountOnPrime)

		require.NoError(t, err)
	})
	//nolint:dupl
	t.Run("9. Both directions sequential and parallel", func(t *testing.T) {
		const (
			sequentialInstances = 5
			parallelInstances   = 6
		)

		prevAmountOnVector, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)
		prevAmountOnPrime, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)

		sendAmount := uint64(1_000_000)

		for j := 0; j < sequentialInstances; j++ {
			var wg sync.WaitGroup

			for i := 0; i < parallelInstances; i++ {
				wg.Add(1)

				go func(run, idx int) {
					defer wg.Done()

					txHash := cardanofw.BridgeAmountFull(t, ctx, txProviderPrime, apex.PrimeCluster.NetworkConfig(),
						apex.PrimeMultisigAddr, apex.VectorMultisigFeeAddr, walletKeysPrime[idx],
						user.VectorAddress, sendAmount)
					fmt.Printf("run: %v. Prime tx %v sent. hash: %s\n", run+1, idx+1, txHash)
				}(j, i)
			}

			wg.Wait()

			for i := 0; i < parallelInstances; i++ {
				wg.Add(1)

				go func(run, idx int) {
					defer wg.Done()

					txHash := cardanofw.BridgeAmountFull(t, ctx, txProviderVector, apex.VectorCluster.NetworkConfig(),
						apex.VectorMultisigAddr, apex.PrimeMultisigFeeAddr, walletKeysVector[idx],
						user.PrimeAddress, sendAmount)
					fmt.Printf("run: %v. Vector tx %v sent. hash: %s\n", run+1, idx+1, txHash)
				}(j, i)
			}

			wg.Wait()
		}

		fmt.Printf("Waiting for %v TXs on vector:\n", sequentialInstances*parallelInstances)

		expectedAmountOnVector := prevAmountOnVector + uint64(sequentialInstances)*uint64(parallelInstances)*sendAmount
		err = cardanofw.WaitForAmount(ctx, txProviderVector, user.VectorAddress, func(val uint64) bool {
			return val >= expectedAmountOnVector
		}, 500, time.Second*2)
		// require.NoError(t, err)
		if err != nil {
			fmt.Printf("ERROR TX ON VECTOR")
		}

		fmt.Printf("%v TXs on vector confirmed\n", sequentialInstances*parallelInstances)

		fmt.Printf("Waiting for %v TXs on prime\n", sequentialInstances*parallelInstances)

		expectedAmountOnPrime := prevAmountOnPrime + uint64(sequentialInstances)*uint64(parallelInstances)*sendAmount
		err = cardanofw.WaitForAmount(ctx, txProviderPrime, user.PrimeAddress, func(val uint64) bool {
			return val >= expectedAmountOnPrime
		}, 500, time.Second*2)

		newAmountOnVector, _ := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		newAmountOnPrime, _ := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)

		fmt.Printf("prevAmountOnVector: %+v, prevAmountOnPrime: %+v\n", prevAmountOnVector, prevAmountOnPrime)
		fmt.Printf("newAmountOnVector:  %+v, newAmountOnPrime:  %+v\n", newAmountOnVector, newAmountOnPrime)

		require.NoError(t, err)
		fmt.Printf("%v TXs on prime confirmed\n", sequentialInstances*parallelInstances)
	})
}

func TestE2E_ApexBridgeCentralized_ValidScenarios_BigTests(t *testing.T) {
	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.SetupAndRunApexCentralizedBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.CreateAndFundUser(t, ctx, uint64(20_000_000_000))

	txProviderPrime := apex.GetPrimeTxProvider()
	txProviderVector := apex.GetVectorTxProvider()

	primeGenesisWallet := apex.GetPrimeGenesisWallet(t)
	vectorGenesisWallet := apex.GetVectorGenesisWallet(t)

	primeGenesisAddress, err := cardanofw.GetAddress(apex.PrimeCluster.NetworkConfig().NetworkType, primeGenesisWallet)
	require.NoError(t, err)

	vectorGenesisAddress, err := cardanofw.GetAddress(apex.VectorCluster.NetworkConfig().NetworkType, vectorGenesisWallet)
	require.NoError(t, err)

	fmt.Println("prime genesis addr: ", primeGenesisAddress)
	fmt.Println("vector genesis addr: ", vectorGenesisAddress)
	fmt.Println("prime user addr: ", user.PrimeAddress)
	fmt.Println("vector user addr: ", user.VectorAddress)
	fmt.Println("prime multisig addr: ", apex.PrimeMultisigAddr)
	fmt.Println("prime fee addr: ", apex.PrimeMultisigFeeAddr)
	fmt.Println("vector multisig addr: ", apex.VectorMultisigAddr)
	fmt.Println("vector fee addr: ", apex.VectorMultisigFeeAddr)

	//nolint:dupl
	t.Run("From prime to vector 200x 5min 90%", func(t *testing.T) {
		instances := 200
		maxWaitTime := 300
		fundSendAmount := uint64(5_000_000)
		sendAmount := uint64(1_000_000)
		successChance := 90 // 90%
		succeededCount := 0
		walletKeys := make([]*wallet.Wallet, instances)

		prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)

		fmt.Printf("Funding %v Wallets\n", instances)

		for i := 0; i < instances; i++ {
			if (i+1)%100 == 0 {
				fmt.Printf("Funded %v..%v\n", i-99, i)
			}

			walletKeys[i], err = wallet.GenerateWallet(false)
			require.NoError(t, err)

			walletAddress, err := cardanofw.GetAddress(apex.PrimeCluster.NetworkConfig().NetworkType, walletKeys[i])
			require.NoError(t, err)
			require.NoError(t, err)

			user.SendToAddress(t, ctx, txProviderPrime, primeGenesisWallet, fundSendAmount, walletAddress.String(), apex.PrimeCluster.NetworkConfig())
		}

		fmt.Printf("Funding Complete\n")
		fmt.Printf("Sending %v transactions in %v seconds\n", instances, maxWaitTime)

		var wg sync.WaitGroup
		for i := 0; i < instances; i++ {
			wg.Add(1)

			go func(idx int) {
				defer wg.Done()

				if successChance > rand.Intn(100) {
					succeededCount++
					sleepTime := rand.Intn(maxWaitTime)
					time.Sleep(time.Second * time.Duration(sleepTime))

					cardanofw.BridgeAmountFull(t, ctx, txProviderPrime, apex.PrimeCluster.NetworkConfig(),
						apex.PrimeMultisigAddr, apex.VectorMultisigFeeAddr, walletKeys[idx], user.VectorAddress, sendAmount)
				} else {
					feeAmount := uint64(1_100_000)
					receivers := map[string]uint64{
						user.VectorAddress: sendAmount * 10, // 10Ada
					}

					bridgingRequestMetadata, err := cardanofw.CreateMetaData(
						user.PrimeAddress, receivers, cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()), feeAmount)
					require.NoError(t, err)

					_, err = cardanofw.SendTx(
						ctx, txProviderPrime, walletKeys[idx], sendAmount+feeAmount, apex.PrimeMultisigAddr,
						apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
					require.NoError(t, err)
				}
			}(i)
		}

		wg.Wait()

		fmt.Printf("All tx sent, waiting for confirmation.\n")

		expectedAmount := prevAmount + uint64(succeededCount)*sendAmount

		var newAmount uint64
		for i := 0; i < instances; i++ {
			newAmount, err = cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
			require.NoError(t, err)
			fmt.Printf("Current Amount Prime: %v. Expected: %v\n", newAmount, expectedAmount)

			if newAmount == expectedAmount {
				break
			}

			time.Sleep(time.Second * 10)
		}

		fmt.Printf("Success count: %v. prevAmount: %v. newAmount: %v. expectedAmount: %v\n", succeededCount, prevAmount, newAmount, expectedAmount)
	})

	//nolint:dupl
	t.Run("From prime to vector 1000x 20min 90%", func(t *testing.T) {
		instances := 1000
		maxWaitTime := 1200
		fundSendAmount := uint64(5_000_000)
		sendAmount := uint64(1_000_000)
		successChance := 90 // 90%
		succeededCount := 0
		walletKeys := make([]*wallet.Wallet, instances)

		prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)

		fmt.Printf("Funding %v Wallets\n", instances)

		for i := 0; i < instances; i++ {
			if (i+1)%100 == 0 {
				fmt.Printf("Funded %v..%v\n", i-99, i)
			}

			walletKeys[i], err = wallet.GenerateWallet(false)
			require.NoError(t, err)

			walletAddress, err := cardanofw.GetAddress(apex.PrimeCluster.NetworkConfig().NetworkType, walletKeys[i])
			require.NoError(t, err)

			user.SendToAddress(t, ctx, txProviderPrime, primeGenesisWallet, fundSendAmount, walletAddress.String(), apex.PrimeCluster.NetworkConfig())
		}

		fmt.Printf("Funding Complete\n")
		fmt.Printf("Sending %v transactions in %v seconds\n", instances, maxWaitTime)

		var wg sync.WaitGroup
		for i := 0; i < instances; i++ {
			wg.Add(1)

			go func(idx int) {
				defer wg.Done()

				if successChance > rand.Intn(100) {
					succeededCount++
					sleepTime := rand.Intn(maxWaitTime)
					time.Sleep(time.Second * time.Duration(sleepTime))

					cardanofw.BridgeAmountFull(t, ctx, txProviderPrime, apex.PrimeCluster.NetworkConfig(),
						apex.PrimeMultisigAddr, apex.VectorMultisigFeeAddr, walletKeys[idx], user.VectorAddress, sendAmount)
				} else {
					feeAmount := uint64(1_100_000)
					receivers := map[string]uint64{
						user.VectorAddress: sendAmount * 10, // 10Ada+
					}

					bridgingRequestMetadata, err := cardanofw.CreateMetaData(
						user.PrimeAddress, receivers, cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()), feeAmount)
					require.NoError(t, err)

					_, err = cardanofw.SendTx(
						ctx, txProviderPrime, walletKeys[idx], sendAmount+feeAmount, apex.PrimeMultisigAddr,
						apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
					require.NoError(t, err)
				}
			}(i)
		}

		wg.Wait()

		fmt.Printf("All tx sent, waiting for confirmation.\n")

		expectedAmount := prevAmount + uint64(succeededCount)*sendAmount

		var newAmount uint64
		for i := 0; i < instances; i++ {
			newAmount, err = cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
			require.NoError(t, err)
			fmt.Printf("Current Amount Prime: %v. Expected: %v\n", newAmount, expectedAmount)

			if newAmount == expectedAmount {
				break
			}

			time.Sleep(time.Second * 10)
		}

		fmt.Printf("Success count: %v. prevAmount: %v. newAmount: %v. expectedAmount: %v\n", succeededCount, prevAmount, newAmount, expectedAmount)
	})

	t.Run("Both directions 1000x 60min 90%", func(t *testing.T) {
		instances := 1000
		maxWaitTime := 3600
		fundSendAmount := uint64(5_000_000)
		sendAmount := uint64(1_000_000)
		successChance := 90 // 90%
		succeededCountPrime := 0
		succeededCountVector := 0
		walletKeysPrime := make([]*wallet.Wallet, instances)
		walletKeysVector := make([]*wallet.Wallet, instances)

		prevAmountPrime, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)
		prevAmountVector, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)

		fmt.Printf("Funding %v Wallets\n", instances*2)

		for i := 0; i < instances; i++ {
			if (i+1)%100 == 0 {
				fmt.Printf("Funded %v..%v on prime\n", i-99, i)
			}

			walletKeysPrime[i], err = wallet.GenerateWallet(false)
			require.NoError(t, err)

			walletAddress, err := cardanofw.GetAddress(apex.PrimeCluster.NetworkConfig().NetworkType, walletKeysPrime[i])
			require.NoError(t, err)

			user.SendToAddress(t, ctx, txProviderPrime, primeGenesisWallet, fundSendAmount, walletAddress.String(), apex.PrimeCluster.NetworkConfig())
		}

		for i := 0; i < instances; i++ {
			if (i+1)%100 == 0 {
				fmt.Printf("Funded %v..%v on vector\n", i-99, i)
			}

			walletKeysVector[i], err = wallet.GenerateWallet(false)
			require.NoError(t, err)

			walletAddress, err := cardanofw.GetAddress(apex.VectorCluster.NetworkConfig().NetworkType, walletKeysVector[i])
			require.NoError(t, err)

			user.SendToAddress(t, ctx, txProviderVector, vectorGenesisWallet, fundSendAmount, walletAddress.String(), apex.VectorCluster.NetworkConfig())
		}

		fmt.Printf("Funding Complete\n")
		fmt.Printf("Sending %v transactions in %v seconds\n", instances*2, maxWaitTime)

		var wg sync.WaitGroup
		for i := 0; i < instances; i++ {
			wg.Add(2)

			//nolint:dupl
			go func(idx int) {
				defer wg.Done()

				if successChance > rand.Intn(100) {
					succeededCountPrime++
					sleepTime := rand.Intn(maxWaitTime)
					time.Sleep(time.Second * time.Duration(sleepTime))

					cardanofw.BridgeAmountFull(t, ctx, txProviderPrime, apex.PrimeCluster.NetworkConfig(),
						apex.PrimeMultisigAddr, apex.VectorMultisigFeeAddr, walletKeysPrime[idx], user.VectorAddress, sendAmount)
				} else {
					feeAmount := uint64(1_100_000)
					receivers := map[string]uint64{
						user.VectorAddress: sendAmount * 10, // 10Ada+
					}

					bridgingRequestMetadata, err := cardanofw.CreateMetaData(
						user.PrimeAddress, receivers, cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()), feeAmount)
					require.NoError(t, err)

					_, err = cardanofw.SendTx(
						ctx, txProviderPrime, walletKeysPrime[idx], sendAmount+feeAmount, apex.PrimeMultisigAddr,
						apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
					require.NoError(t, err)
				}
			}(i)

			//nolint:dupl
			go func(idx int) {
				defer wg.Done()

				if successChance > rand.Intn(100) {
					succeededCountVector++
					sleepTime := rand.Intn(maxWaitTime)
					time.Sleep(time.Second * time.Duration(sleepTime))

					cardanofw.BridgeAmountFull(t, ctx, txProviderVector, apex.VectorCluster.NetworkConfig(),
						apex.VectorMultisigAddr, apex.PrimeMultisigFeeAddr, walletKeysVector[idx], user.PrimeAddress, sendAmount)
				} else {
					feeAmount := uint64(1_100_000)
					receivers := map[string]uint64{
						user.PrimeAddress: sendAmount * 10, // 10Ada+
					}

					bridgingRequestMetadata, err := cardanofw.CreateMetaData(
						user.VectorAddress, receivers, cardanofw.GetDestinationChainID(apex.VectorCluster.NetworkConfig()), feeAmount)
					require.NoError(t, err)

					_, err = cardanofw.SendTx(
						ctx, txProviderVector, walletKeysVector[idx], sendAmount+feeAmount, apex.VectorMultisigAddr,
						apex.VectorCluster.NetworkConfig(), bridgingRequestMetadata)
					require.NoError(t, err)
				}
			}(i)
		}

		wg.Wait()

		fmt.Printf("All tx sent, waiting for confirmation.\n")

		expectedAmountPrime := prevAmountPrime + uint64(succeededCountPrime)*sendAmount
		expectedAmountVector := prevAmountVector + uint64(succeededCountVector)*sendAmount

		for i := 0; i < instances; i++ {
			newAmountPrime, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
			require.NoError(t, err)
			fmt.Printf("Current Amount Prime: %v. Expected: %v\n", newAmountPrime, expectedAmountPrime)

			newAmountVector, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
			require.NoError(t, err)
			fmt.Printf("Current Amount Vector: %v. Expected: %v\n", newAmountVector, expectedAmountVector)

			if newAmountPrime == expectedAmountPrime && newAmountVector == expectedAmountVector {
				break
			}

			time.Sleep(time.Second * 10)
		}

		newAmountPrime, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)
		fmt.Printf("Success count: %v. prevAmount: %v. newAmount: %v. expectedAmount: %v\n", succeededCountPrime, prevAmountPrime, newAmountPrime, expectedAmountPrime)

		newAmountVector, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)
		fmt.Printf("Success count: %v. prevAmount: %v. newAmount: %v. expectedAmount: %v\n", succeededCountVector, prevAmountVector, newAmountVector, expectedAmountVector)
	})
}

func TestE2E_ApexBridgeCentralized_OneTx(t *testing.T) {
	const (
		apiKey               = "test_api_key"
		maxParallelInstances = 5
		fundAmount           = uint64(50_000_000_000)
	)

	var (
		err                error
		walletKeysPrime    = make([]*wallet.Wallet, maxParallelInstances)
		walletKeysVector   = make([]*wallet.Wallet, maxParallelInstances)
		primeClusterConfig = &cardanofw.RunCardanoClusterConfig{
			ID:                 0,
			NodesCount:         4,
			NetworkType:        wallet.TestNetNetwork,
			InitialFundsAmount: fundAmount,
			InitialFundsKeys:   make([]string, maxParallelInstances),
		}
		vectorClusterConfig = &cardanofw.RunCardanoClusterConfig{
			ID:                 1,
			NodesCount:         4,
			NetworkType:        wallet.VectorTestNetNetwork,
			InitialFundsAmount: fundAmount,
			InitialFundsKeys:   make([]string, maxParallelInstances),
		}
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	for i := range walletKeysPrime {
		walletKeysPrime[i], err = wallet.GenerateWallet(false)
		require.NoError(t, err)

		walletKeysVector[i], err = wallet.GenerateWallet(false)
		require.NoError(t, err)

		addrPrime, err := wallet.NewEnterpriseAddress(primeClusterConfig.NetworkType, walletKeysPrime[i].VerificationKey)
		require.NoError(t, err)

		addrVec, err := wallet.NewEnterpriseAddress(vectorClusterConfig.NetworkType, walletKeysVector[i].VerificationKey)
		require.NoError(t, err)

		primeClusterConfig.InitialFundsKeys[i] = hex.EncodeToString(addrPrime.Bytes())
		vectorClusterConfig.InitialFundsKeys[i] = hex.EncodeToString(addrVec.Bytes())
	}

	apex := cardanofw.SetupAndRunApexCentralizedBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithPrimeClusterConfig(primeClusterConfig),
		cardanofw.WithVectorClusterConfig(vectorClusterConfig),
	)

	// defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.CreateAndFundUser(t, ctx, uint64(20_000_000_000))

	txProviderPrime := apex.GetPrimeTxProvider()
	txProviderVector := apex.GetVectorTxProvider()

	primeGenesisWallet := apex.GetPrimeGenesisWallet(t)
	vectorGenesisWallet := apex.GetVectorGenesisWallet(t)

	primeGenesisAddress, err := cardanofw.GetAddress(apex.PrimeCluster.NetworkConfig().NetworkType, primeGenesisWallet)
	require.NoError(t, err)

	vectorGenesisAddress, err := cardanofw.GetAddress(apex.VectorCluster.NetworkConfig().NetworkType, vectorGenesisWallet)
	require.NoError(t, err)

	fmt.Println("prime genesis addr: ", primeGenesisAddress)
	fmt.Println("vector genesis addr: ", vectorGenesisAddress)
	fmt.Println("prime user addr: ", user.PrimeAddress)
	fmt.Println("vector user addr: ", user.VectorAddress)
	fmt.Println("prime multisig addr: ", apex.PrimeMultisigAddr)
	fmt.Println("prime fee addr: ", apex.PrimeMultisigFeeAddr)
	fmt.Println("vector multisig addr: ", apex.VectorMultisigAddr)
	fmt.Println("vector fee addr: ", apex.VectorMultisigFeeAddr)

	fmt.Println("network magic prime: ", apex.PrimeCluster.Config.NetworkMagic)
	fmt.Println("network magic vectr: ", apex.VectorCluster.Config.NetworkMagic)

	fmt.Println("SocketPath prime: ", apex.PrimeCluster.OgmiosServer.SocketPath())
	fmt.Println("SocketPath vectr: ", apex.VectorCluster.OgmiosServer.SocketPath())

	/*
		t.Run("1. From prime to vector one - wait for other side", func(t *testing.T) {
			t.Skip()
			const (
				sendAmount = uint64(1_000_000)
				instances  = 1
			)

			for i := 0; i < instances; i++ {
				prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
				require.NoError(t, err)

				fmt.Printf("%v - prevAmount %v\n", i+1, prevAmount)

				txHash := user.BridgeAmount(t, ctx, txProviderPrime, apex.PrimeMultisigAddr,
					apex.VectorMultisigFeeAddr, sendAmount, apex.PrimeCluster.NetworkConfig())

				fmt.Printf("%v - Tx sent. hash: %s\n", i+1, txHash)

				expectedAmount := prevAmount + sendAmount
				fmt.Printf("%v - expectedAmount %v\n", i+1, expectedAmount)

				err = cardanofw.WaitForAmount(ctx, txProviderVector, user.VectorAddress, func(val uint64) bool {
					return val == expectedAmount
				}, 20, time.Second*10)
				// require.NoError(t, err)
			}

			signalChannel := make(chan os.Signal, 1)
			// Notify the signalChannel when the interrupt signal is received (Ctrl+C)
			signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)

			<-signalChannel
		})

		t.Run("4. From vector to prime one by one", func(t *testing.T) {
			t.Skip()
			const (
				sendAmount = uint64(1_000_000)
				instances  = 2
			)

			prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
			require.NoError(t, err)

			for i := 0; i < instances; i++ {
				txHash := user.BridgeAmount(t, ctx, txProviderVector, apex.VectorMultisigAddr,
					apex.PrimeMultisigFeeAddr, sendAmount, apex.VectorCluster.NetworkConfig())

				fmt.Printf("Tx %v sent. hash: %s\n", i+1, txHash)
			}

			expectedAmount := prevAmount + uint64(instances)*sendAmount
			err = cardanofw.WaitForAmount(ctx, txProviderPrime, user.PrimeAddress, func(val uint64) bool {
				fmt.Printf("DN_LOG_TAG WaitForAmount val: %+v, prevAmount: %+v, expectedAmount: %+v \n", val, prevAmount, expectedAmount)
				return val == expectedAmount
				// return prevAmount != val
			}, 100, time.Second*5)
			// require.NoError(t, err)
			if err != nil {
				fmt.Printf("err: %+v\n", err)
			}

			signalChannel := make(chan os.Signal, 1)
			// Notify the signalChannel when the interrupt signal is received (Ctrl+C)
			signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)

			<-signalChannel
		})

		t.Run("5. From vector to prime one by one", func(t *testing.T) {
			t.Skip()
			const (
				sendAmount = uint64(1_000_000)
				instances  = 2
			)

			prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
			require.NoError(t, err)

			for i := 0; i < instances; i++ {
				txHash := user.BridgeAmount(t, ctx, txProviderVector, apex.VectorMultisigAddr,
					apex.PrimeMultisigFeeAddr, sendAmount, apex.VectorCluster.NetworkConfig())

				fmt.Printf("Tx %v sent. hash: %s\n", i+1, txHash)

				expectedAmount := prevAmount + uint64(i+1)*sendAmount
				err = cardanofw.WaitForAmount(ctx, txProviderPrime, user.PrimeAddress, func(val uint64) bool {
					fmt.Printf("DN_LOG_TAG WaitForAmount val: %+v, prevAmount: %+v, expectedAmount: %+v \n", val, prevAmount, expectedAmount)
					return val == expectedAmount
					// return prevAmount != val
				}, 100, time.Second*5)
				// require.NoError(t, err)
				if err != nil {
					fmt.Printf("err: %+v\n", err)
				}
			}

			signalChannel := make(chan os.Signal, 1)
			// Notify the signalChannel when the interrupt signal is received (Ctrl+C)
			signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)

			<-signalChannel
		})
	*/

	t.Run("0. From vector to prime, ada to nt -> prime to vector, nt to ada", func(t *testing.T) {
		t.Skip()
		//WORKS
		const (
			sendAmount = uint64(2_000_000)
			instances  = 2
		)

		// vector ada -> prime NT
		prevAmount, err := cardanofw.GetNativeTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)

		for i := 0; i < instances; i++ {
			// DN_TODO: FIX expected amount should be sendAmount - minUtxoValue of tokens
			// DN_TODO: FIX hardcoded value of 10 tokens minted
			txHash := user.BridgeAmount(t, ctx, txProviderVector, apex.VectorMultisigAddr,
				apex.PrimeMultisigFeeAddr, sendAmount, apex.VectorCluster.NetworkConfig())

			print("Tx %v sent. hash: %s\n", i+1, txHash)

			expectedAmount := prevAmount + uint64(i+1)*uint64(1500000)
			err = cardanofw.WaitForNativeTokenAmount(ctx, txProviderPrime, user.PrimeAddress, func(val uint64) bool {
				print("WaitForAmount val: %+v, prevAmount: %+v, expectedAmount: %+v", val, prevAmount, expectedAmount)
				return val == expectedAmount
			}, 100, time.Second*5)
			require.NoError(t, err)
		}

		// prime NT -> vector ADA
		prevAmountNativeTokensPrime, err := cardanofw.GetNativeTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)
		print("PRIME USER NATIVE TOKEN AMOUNT: %+v\n", prevAmountNativeTokensPrime)

		prevAmountLovVector, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)

		sendAmount2 := uint64(1_000_000)

		primeKey := apex.GetPrimeMSigVerificationKey()

		keyDecoded, err := hex.DecodeString(primeKey)
		require.NoError(t, err)

		keyHash, err := wallet.GetKeyHash(keyDecoded)
		require.NoError(t, err)

		policyScript := wallet.PolicyScript{
			Type:    wallet.PolicyScriptSigType,
			KeyHash: keyHash,
		}

		for i := 0; i < 5; i++ {
			txHash := user.BridgeNativeTokenAmount(t, ctx, txProviderPrime, apex.PrimeMultisigAddr,
				apex.VectorMultisigFeeAddr, sendAmount2, apex.PrimeCluster.NetworkConfig(), user.VectorAddress, policyScript)

			print("Tx %v sent. hash: %s\n", i+1, txHash)

			expectedAmount := prevAmountLovVector + uint64(i+1)*sendAmount2
			err = cardanofw.WaitForAmount(ctx, txProviderVector, user.VectorAddress, func(val uint64) bool {
				print("WaitForAmount val: %+v, prevAmount: %+v, expectedAmount: %+v", val, prevAmountLovVector, expectedAmount)

				// primeMSANativeTokens, err := cardanofw.GetNativeTokenAmount(ctx, txProviderPrime, apex.PrimeMultisigAddr)
				// require.NoError(t, err)
				// print("PRIME MSG NATIVE TOKEN AMOUNT: %+v\n", primeMSANativeTokens)

				// primeNativeTokens, err := cardanofw.GetNativeTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
				// require.NoError(t, err)
				// print("PRIME USER NATIVE TOKEN AMOUNT: %+v\n", primeNativeTokens)
				return val == expectedAmount
			}, 100, time.Second*5)
			// require.NoError(t, err)
			if err != nil {
				fmt.Printf("ERROR %+v\n", err)
			}
		}

		primeMSANativeTokens, err := cardanofw.GetNativeTokenAmount(ctx, txProviderPrime, apex.PrimeMultisigAddr)
		require.NoError(t, err)
		print("PRIME MSG NATIVE TOKEN AMOUNT: %+v\n", primeMSANativeTokens)

		primeNativeTokens, err := cardanofw.GetNativeTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)
		print("PRIME USER NATIVE TOKEN AMOUNT: %+v\n", primeNativeTokens)

		// signalChannel := make(chan os.Signal, 1)
		// // Notify the signalChannel when the interrupt signal is received (Ctrl+C)
		// signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)

		// <-signalChannel
	})

	t.Run("0. From vector to prime parallel", func(t *testing.T) {
		t.Skip()
		//WORKS
		const (
			instances  = 5
			sendAmount = uint64(2_000_000)
		)

		// vector ada -> prime NT
		prevAmount, err := cardanofw.GetNativeTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)

		vectorMSANativeTokens, err := cardanofw.GetTokenAmount(ctx, txProviderVector, apex.VectorMultisigAddr)
		require.NoError(t, err)
		print("VECTOR MSG NATIVE TOKEN AMOUNT: %+v\n", vectorMSANativeTokens)

		var wg sync.WaitGroup
		for i := 0; i < instances; i++ {
			wg.Add(1)

			go func(idx int) {
				defer wg.Done()

				// txHash := user.BridgeAmount(t, ctx, txProviderVector, apex.VectorMultisigAddr,
				// apex.PrimeMultisigFeeAddr, sendAmount, apex.VectorCluster.NetworkConfig())

				txHash := cardanofw.BridgeAmountFull(t, ctx, txProviderVector, apex.VectorCluster.NetworkConfig(),
					apex.VectorMultisigAddr, apex.PrimeMultisigFeeAddr, walletKeysVector[idx],
					user.PrimeAddress, sendAmount)

				print("Tx %v sent. hash: %s\n", idx+1, txHash)
			}(i)
		}

		wg.Wait()

		expectedAmount := prevAmount + instances*10

		err = cardanofw.WaitForNativeTokenAmount(ctx, txProviderPrime, user.PrimeAddress, func(val uint64) bool {
			print("WaitForAmount val: %+v, prevAmount: %+v, expectedAmount: %+v", val, prevAmount, expectedAmount)
			return val == expectedAmount
		}, 100, time.Second*5)

		// require.NoError(t, err)
		if err != nil {
			fmt.Printf("ERROR %+v", err)
		}
		fmt.Printf("%v TXs confirmed\n", instances)

		vectorMSANativeTokens, err = cardanofw.GetTokenAmount(ctx, txProviderVector, apex.VectorMultisigAddr)
		require.NoError(t, err)
		print("VECTOR MSG ADA AMOUNT: %+v\n", vectorMSANativeTokens)

		primeMSANativeTokens, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)
		print("PRIME USER ADA AMOUNT: %+v\n", primeMSANativeTokens)

	})

	t.Run("1. From vector to prime one by one - wait for the other side", func(t *testing.T) {
		t.Skip()
		const (
			sendAmount = uint64(1_000_000)
			instances  = 2
		)

		// vector ada -> prime NT
		prevAmount, err := cardanofw.GetNativeTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)

		for i := 0; i < instances; i++ {
			// DN_TODO: FIX expected amount should be sendAmount - minUtxoValue of tokens
			// DN_TODO: FIX hardcoded value of 10 tokens minted
			txHash := user.BridgeAmount(t, ctx, txProviderVector, apex.VectorMultisigAddr,
				apex.PrimeMultisigFeeAddr, sendAmount, apex.VectorCluster.NetworkConfig())

			print("Tx %v sent. hash: %s\n", i+1, txHash)

			// expectedAmount := prevAmount + uint64(i+1)*10
			expectedAmount := prevAmount + uint64(i+1)*uint64(1500000)
			err = cardanofw.WaitForNativeTokenAmount(ctx, txProviderPrime, user.PrimeAddress, func(val uint64) bool {
				print("WaitForAmount val: %+v, prevAmount: %+v, expectedAmount: %+v", val, prevAmount, expectedAmount)
				return val == expectedAmount
			}, 100, time.Second*5)
			require.NoError(t, err)
		}

		// prime NT -> vector ADA
		prevAmountNativeTokensPrime, err := cardanofw.GetNativeTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)
		print("PRIME USER NATIVE TOKEN AMOUNT: %+v\n", prevAmountNativeTokensPrime)
	})

	t.Run("2. From vector to prime one by one", func(t *testing.T) {
		t.Skip()
		const (
			sendAmount = uint64(2_000_000)
			instances  = 2
		)

		// vector ada -> prime NT
		prevAmount, err := cardanofw.GetNativeTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)

		for i := 0; i < instances; i++ {
			// DN_TODO: FIX expected amount should be sendAmount - minUtxoValue of tokens
			// DN_TODO: FIX hardcoded value of 10 tokens minted
			txHash := user.BridgeAmount(t, ctx, txProviderVector, apex.VectorMultisigAddr,
				apex.PrimeMultisigFeeAddr, sendAmount, apex.VectorCluster.NetworkConfig())

			print("Tx %v sent. hash: %s\n", i+1, txHash)
		}

		expectedAmount := prevAmount + instances*uint64(1500000)
		err = cardanofw.WaitForNativeTokenAmount(ctx, txProviderPrime, user.PrimeAddress, func(val uint64) bool {
			print("WaitForAmount val: %+v, prevAmount: %+v, expectedAmount: %+v", val, prevAmount, expectedAmount)
			return val == expectedAmount
		}, 100, time.Second*5)
		require.NoError(t, err)

		// prime NT -> vector ADA
		prevAmountNativeTokensPrime, err := cardanofw.GetNativeTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)
		print("PRIME USER NATIVE TOKEN AMOUNT: %+v\n", prevAmountNativeTokensPrime)
	})

	t.Run("3. From vector to prime parallel", func(t *testing.T) {
		t.Skip()
		const (
			instances  = 5
			sendAmount = uint64(2_000_000)
		)

		// vector ada -> prime NT
		prevAmount, err := cardanofw.GetNativeTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)

		// vectorMSANativeTokens, err := cardanofw.GetTokenAmount(ctx, txProviderVector, apex.VectorMultisigAddr)
		// require.NoError(t, err)
		// print("VECTOR MSG NATIVE TOKEN AMOUNT: %+v\n", vectorMSANativeTokens)

		var wg sync.WaitGroup
		for i := 0; i < instances; i++ {
			wg.Add(1)

			go func(idx int) {
				defer wg.Done()

				txHash := cardanofw.BridgeAmountFull(t, ctx, txProviderVector, apex.VectorCluster.NetworkConfig(),
					apex.VectorMultisigAddr, apex.PrimeMultisigFeeAddr, walletKeysVector[idx],
					user.PrimeAddress, sendAmount)

				print("Tx %v sent. hash: %s\n", idx+1, txHash)
			}(i)
		}

		wg.Wait()

		expectedAmount := prevAmount + instances*sendAmount

		err = cardanofw.WaitForNativeTokenAmount(ctx, txProviderPrime, user.PrimeAddress, func(val uint64) bool {
			print("WaitForAmount val: %+v, prevAmount: %+v, expectedAmount: %+v", val, prevAmount, expectedAmount)
			return val == expectedAmount
		}, 100, time.Second*5)

		require.NoError(t, err)
		fmt.Printf("%v TXs confirmed\n", instances)

		// vectorMSANativeTokens, err = cardanofw.GetTokenAmount(ctx, txProviderVector, apex.VectorMultisigAddr)
		// require.NoError(t, err)
		// print("VECTOR MSG ADA AMOUNT: %+v\n", vectorMSANativeTokens)

		// primeMSANativeTokens, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		// require.NoError(t, err)
		// print("PRIME USER ADA AMOUNT: %+v\n", primeMSANativeTokens)
	})

}

func print(format string, a ...any) {

	fmt.Printf("\033[35mDN_LOG_TAG\033[0m "+format+"\n", a...)
}

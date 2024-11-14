package e2e

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	infrawallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

const (
	receiver    = "addr_test1wzmg58l2fnzz3jgweth2mldyemtcfgz0ralxf9mduje59mctm89mm"
	networkType = infrawallet.TestNetNetwork
	sendAmount  = infrawallet.MinUTxODefaultValue + uint64(200)
)

func BenchmarkOgmiosTest_Transactions(b *testing.B) {
	const (
		nodesCount  = 3
		maxWallets  = 2000
		txPerWallet = 10
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	wallets := make([]infrawallet.IWallet, maxWallets)
	addresses := make([]infrawallet.CardanoAddress, maxWallets)

	for i := range wallets {
		w, err := infrawallet.GenerateWallet(true)
		require.NoError(b, err)

		baseAddress, err := infrawallet.NewBaseAddress(
			networkType, w.GetVerificationKey(), w.GetStakeVerificationKey())
		require.NoError(b, err)

		wallets[i] = w
		addresses[i] = baseAddress
	}

	cardanoChainConfig := cardanofw.NewPrimeChainConfig()
	cardanoChainConfig.NetworkType = networkType
	cardanoChainConfig.NodesCount = nodesCount
	cardanoChainConfig.PremineAmount = uint64(2_000_000_000_000_000_000)
	cardanoChainConfig.PreminesAddresses = make([]string, maxWallets)

	for i, v := range addresses {
		cardanoChainConfig.PreminesAddresses[i] = hex.EncodeToString(v.Bytes())
	}

	cardanoChain := cardanofw.NewTestCardanoChain(cardanoChainConfig)
	apexSystem := &cardanofw.ApexSystem{}

	require.NoError(b, cardanoChain.RunChain(b))

	cardanoChain.PopulateApexSystem(apexSystem)

	b.Cleanup(func() {
		cardanoChain.Stop()
	})

	txProvider := infrawallet.NewTxProviderOgmios(apexSystem.PrimeInfo.OgmiosURL)

	// b.Run("Sequential", func(b *testing.B) {
	// 	successAll, failedAll := uint64(0), uint64(0)

	// 	for i := 0; i < b.N; i++ {
	// 		s, f := RunSequential(ctx, time.Second*4, txProvider, wallets[:200])
	// 		b.Logf("iteration %d) success: %d, failed: %d", i, s, f)
	// 		fmt.Printf("iteration %d: success: %d, failed: %d\n", i, s, f)

	// 		atomic.AddUint64(&failedAll, f)
	// 		atomic.AddUint64(&successAll, s)
	// 	}

	// 	s, f := atomic.LoadUint64(&successAll), atomic.LoadUint64(&failedAll)
	// 	b.Logf("aggregate) success: %f, failed: %f", float64(s)/float64(b.N), float64(f)/float64(b.N))
	// 	fmt.Printf("aggregate) success: %f, failed: %f\n", float64(s)/float64(b.N), float64(f)/float64(b.N))
	// })

	// time.Sleep(10 * time.Second)

	b.Run("Parallell with delay 10 milis 200 wallets", func(b *testing.B) {
		successAll, failedAll := uint64(0), uint64(0)

		for i := 0; i < b.N; i++ {
			s, f := RunInParallel(ctx, time.Millisecond*10, txProvider, wallets[:200])
			b.Logf("iteration %d) success: %d, failed: %d", i, s, f)
			fmt.Printf("iteration %d: success: %d, failed: %d\n", i, s, f)

			atomic.AddUint64(&failedAll, f)
			atomic.AddUint64(&successAll, s)
		}

		s, f := atomic.LoadUint64(&successAll), atomic.LoadUint64(&failedAll)
		b.Logf("aggregate) success: %f, failed: %f", float64(s)/float64(b.N), float64(f)/float64(b.N))
		fmt.Printf("aggregate) success: %f, failed: %f\n", float64(s)/float64(b.N), float64(f)/float64(b.N))
	})
}

func RunInParallel(
	ctx context.Context, waitInitial time.Duration,
	txProvider infrawallet.ITxProvider, wallets []infrawallet.IWallet,
) (uint64, uint64) {
	wg := sync.WaitGroup{}
	wg.Add(len(wallets))

	success, failed := uint64(0), uint64(0)

	for j, wallet := range wallets {
		go func(wait time.Duration, wallet infrawallet.IWallet) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}

			_, err := cardanofw.SendCardanoTransaction(
				ctx, txProvider, wallet,
				sendAmount, receiver, networkType, nil)
			if infracommon.IsContextDoneErr(err) {
				return
			}

			if err != nil {
				atomic.AddUint64(&failed, 1)
			} else {
				atomic.AddUint64(&success, 1)
			}
		}(waitInitial*time.Duration(j), wallet)
	}

	wg.Wait()

	return atomic.LoadUint64(&success), atomic.LoadUint64(&failed)
}

func RunSequential(
	ctx context.Context, timeout time.Duration,
	txProvider infrawallet.ITxProvider, wallets []infrawallet.IWallet,
) (uint64, uint64) {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	success, failed := uint64(0), uint64(0)
	walletID := -1

	for j := 0; j < 1_000_000_000; j++ {
		walletID = (walletID + 1) % len(wallets)

		_, err := cardanofw.SendCardanoTransaction(
			ctxWithTimeout, txProvider, wallets[walletID],
			sendAmount, receiver, networkType, nil)
		if err != nil {
			if infracommon.IsContextDoneErr(err) {
				break
			}

			failed++
		} else {
			success++
		}
	}

	return success, failed
}

func RunInParallelAll(
	ctx context.Context, txProvider infrawallet.ITxProvider, wallets []infrawallet.IWallet,
) (uint64, uint64) {
	success, failed := uint64(0), uint64(0)

	wg := sync.WaitGroup{}
	wg.Add(len(wallets))

	for i, w := range wallets {
		go func(idx int, wallet infrawallet.IWallet) {
			defer wg.Done()

			_, err := cardanofw.SendCardanoTransaction(
				ctx, txProvider, wallet, sendAmount, receiver, networkType, nil)
			if err != nil {
				atomic.AddUint64(&failed, 1)
			} else {
				atomic.AddUint64(&success, 1)
			}
		}(i, w)
	}

	wg.Wait()

	return atomic.LoadUint64(&success), atomic.LoadUint64(&failed)
}

func RunInParallelAllReceipt(
	ctx context.Context, txProvider infrawallet.ITxProvider, wallets []infrawallet.IWallet,
) (uint64, uint64) {
	success, failed := uint64(0), uint64(0)

	wg := sync.WaitGroup{}
	wg.Add(len(wallets))

	for i, w := range wallets {
		go func(idx int, wallet infrawallet.IWallet) {
			defer wg.Done()

			txHash, err := cardanofw.SendCardanoTransaction(
				ctx, txProvider, wallet, sendAmount, receiver, networkType, nil)
			if err != nil {
				atomic.AddUint64(&failed, 1)

				return
			}

			_, err = infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (bool, error) {
				found, err := infrawallet.IsTxInUtxos(ctx, txProvider, receiver, txHash)
				if err != nil {
					return false, err
				} else if !found {
					return false, infracommon.ErrRetryTryAgain
				}

				return true, nil
			})
			if err != nil {
				atomic.AddUint64(&failed, 1)
			} else {
				atomic.AddUint64(&success, 1)
			}
		}(i, w)
	}

	wg.Wait()

	return atomic.LoadUint64(&success), atomic.LoadUint64(&failed)
}

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

const (
	BaseURL = "http://localhost:10000/api/retrieve"
	APIKey  = "zum-zum-eprom"
)

type Result struct {
	Endpoint string
	Success  int
	Failure  int
	Time     time.Duration
	Response []string
}

func callGetEndpoint(request string, callCount int, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()

	client := &http.Client{}

	success, failure := 0, 0
	start := time.Now().UTC()
	response := make([]string, callCount)

	for i := 0; i < callCount; i++ {
		req, _ := http.NewRequest("GET", request, nil)
		req.Header.Set("x-api-key", APIKey)
		resp, err := client.Do(req)

		if err != nil || resp.StatusCode != http.StatusOK {
			failure++
		} else {
			res, err := io.ReadAll(resp.Body)
			resp.Body.Close()

			if err != nil {
				failure++
			} else {
				success++
				response[i] = string(res)
			}
		}
	}

	time := time.Since(start)

	results <- Result{Endpoint: request, Success: success, Failure: failure, Time: time, Response: response}
}

func getFunc(function, params string) string {
	return fmt.Sprintf("%s/%s/%s", BaseURL, function, params)
}

func RunBenchmarks(users []*cardanofw.TestApexUser, prime cardanofw.CardanoChainInfo, requestCount int) chan Result {
	results := make(chan Result, 4)
	address := users[0].PrimeAddress.String()

	var wg sync.WaitGroup

	wg.Add(3)

	go callGetEndpoint(getFunc("utxo", address), requestCount, results, &wg)
	go callGetEndpoint(getFunc("protocol-params", ""), requestCount, results, &wg)
	go callGetEndpoint(getFunc("tip", ""), requestCount, results, &wg)
	// go callSendTx(users, prime, requestCount, results, &wg)

	wg.Wait()

	close(results)

	return results
}

func callSendTx(users []*cardanofw.TestApexUser, prime cardanofw.CardanoChainInfo, callCount int,
	results chan<- Result, wg *sync.WaitGroup,
) {
	ctx := context.Background()

	provider := prime.GetTxProvider()
	magic := cardanofw.GetNetworkMagic(wallet.TestNetNetwork)

	tip, err := provider.GetTip(ctx)
	if err != nil {
		fmt.Println(err)
	}

	ttl := tip.Slot + 1000

	pp, err := provider.GetProtocolParameters(ctx)
	if err != nil {
		fmt.Println(err)
	}

	cardanoCliBinary := "cardano-cli"

	amount := wallet.MinUTxODefaultValue
	senderAddr := users[0].PrimeAddress.String()
	senderWallet := users[0].PrimeWallet
	receiverAddr := users[1].PrimeAddress.String()

	defer wg.Done()
	url := fmt.Sprintf("%s/submit/tx", BaseURL)
	client := &http.Client{}

	success, failure := 0, 0
	start := time.Now()
	response := make([]string, callCount)

	for i := 0; i < callCount; i++ {
		outputs := []wallet.TxOutput{
			{
				Addr:   receiverAddr,
				Amount: amount,
			},
		}
		desiredSum := amount

		inputs, err := wallet.GetUTXOsForAmount(ctx, provider, senderAddr, desiredSum, desiredSum)
		if err != nil {
			fmt.Println(err)
		}

		rawTx, txHash, err := cardanofw.CreateTx(cardanoCliBinary, magic, pp, ttl, nil, outputs, inputs, senderAddr)
		if err != nil {
			fmt.Println(err)
		}

		witness, err := wallet.CreateTxWitness(txHash, senderWallet)
		if err != nil {
			fmt.Println(err)
		}

		signedTx, err := cardanofw.AssembleTxWitnesses(cardanoCliBinary, rawTx, [][]byte{witness})
		if err != nil {
			fmt.Println(err)
		}

		req, _ := http.NewRequest("POST", url, bytes.NewReader(signedTx))
		req.Header.Set("x-api-key", APIKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)

		if err != nil || resp.StatusCode != http.StatusOK {
			failure++
		} else {
			res, err := io.ReadAll(resp.Body)
			resp.Body.Close()

			if err != nil {
				failure++
			} else {
				success++
				response[i] = string(res)
			}
		}
	}

	results <- Result{Endpoint: url, Success: success, Failure: failure, Time: time.Since(start)}

}

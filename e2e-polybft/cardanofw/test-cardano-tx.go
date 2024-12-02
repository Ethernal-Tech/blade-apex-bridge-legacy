package cardanofw

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

const (
	potentialFee     = 250_000
	ttlSlotNumberInc = 500
)

func SendTx(ctx context.Context,
	txProvider wallet.ITxProvider,
	cardanoWallet *wallet.Wallet,
	amount uint64,
	receiver string,
	networkConfig TestCardanoNetworkConfig,
	metadata []byte,
) (res string, err error) {
	err = ExecuteWithRetryIfNeeded(ctx, func() error {
		res, err = sendTx(ctx, txProvider, cardanoWallet, amount, receiver, networkConfig, metadata)

		return err
	})

	return res, err
}

func SendTxNativeTokens(ctx context.Context,
	txProvider wallet.ITxProvider,
	cardanoWallet *wallet.Wallet,
	amount uint64,
	receiver string,
	networkConfig TestCardanoNetworkConfig,
	metadata []byte,
	policyScript wallet.PolicyScript,
) (res string, err error) {
	err = ExecuteWithRetryIfNeeded(ctx, func() error {
		res, err = sendTxNativeTokens(ctx, txProvider, cardanoWallet, amount, receiver, networkConfig, metadata, policyScript)

		return err
	})

	return res, err
}

func sendTx(ctx context.Context,
	txProvider wallet.ITxProvider,
	cardanoWallet *wallet.Wallet,
	amount uint64,
	receiver string,
	networkConfig TestCardanoNetworkConfig,
	metadata []byte,
) (string, error) {
	caddr, err := GetAddress(networkConfig.NetworkType, cardanoWallet)
	if err != nil {
		return "", err
	}

	cardanoWalletAddr := caddr.String()
	networkTestMagic := networkConfig.NetworkMagic
	cardanoCliBinary := ResolveCardanoCliBinary(networkConfig.NetworkType)

	protocolParams, err := txProvider.GetProtocolParameters(ctx)
	if err != nil {
		return "", err
	}

	qtd, err := txProvider.GetTip(ctx)
	if err != nil {
		return "", err
	}

	fmt.Printf("DN_LOG_TAG receiver addr: %s\n", receiver)
	outputs := []wallet.TxOutput{
		{
			Addr:   receiver,
			Amount: amount,
		},
	}
	desiredSum := amount + potentialFee + wallet.MinUTxODefaultValue

	// inputs, err := wallet.GetUTXOsForAmount(
	// 	ctx, txProvider, cardanoWalletAddr, "TestToken", desiredSum, desiredSum)
	// if err != nil {
	// 	return "", err
	// }

	inputs, err := wallet.GetUTXOsForAmount(
		ctx,
		txProvider,
		cardanoWalletAddr,
		[]string{wallet.AdaTokenName},
		map[string]uint64{wallet.AdaTokenName: desiredSum},
		map[string]uint64{wallet.AdaTokenName: desiredSum})
	if err != nil {
		return "", err
	}

	rawTx, txHash, err := CreateTx(
		cardanoCliBinary,
		networkTestMagic, protocolParams,
		qtd.Slot+ttlSlotNumberInc, metadata,
		outputs, inputs, cardanoWalletAddr)
	if err != nil {
		return "", err
	}

	witness, err := wallet.CreateTxWitness(txHash, cardanoWallet)
	if err != nil {
		return "", err
	}

	signedTx, err := AssembleTxWitnesses(cardanoCliBinary, rawTx, [][]byte{witness})
	if err != nil {
		return "", err
	}

	return txHash, txProvider.SubmitTx(ctx, signedTx)
}

func sendTxNativeTokens(ctx context.Context,
	txProvider wallet.ITxProvider,
	cardanoWallet *wallet.Wallet,
	amount uint64,
	receiver string,
	networkConfig TestCardanoNetworkConfig,
	metadata []byte,
	policyScript wallet.PolicyScript,
) (string, error) {
	caddr, err := GetAddress(networkConfig.NetworkType, cardanoWallet)
	if err != nil {
		return "", err
	}

	cardanoWalletAddr := caddr.String()
	networkTestMagic := networkConfig.NetworkMagic
	cardanoCliBinary := ResolveCardanoCliBinary(networkConfig.NetworkType)

	protocolParams, err := txProvider.GetProtocolParameters(ctx)
	if err != nil {
		return "", err
	}

	qtd, err := txProvider.GetTip(ctx)
	if err != nil {
		return "", err
	}

	testTokenName := "54657374746F6B656E"

	policyID, err := wallet.NewCliUtils(cardanoCliBinary).GetPolicyID(policyScript)
	if err != nil {
		return "", err
	}

	fmt.Printf("AMOUNT: %d\n", amount)
	outputs := []wallet.TxOutput{
		{
			Addr:   receiver,
			Amount: amount,
			Tokens: []wallet.TokenAmount{
				{
					PolicyID: policyID,
					Name:     testTokenName,
					Amount:   uint64(10),
				},
			},
		},
	}
	desiredSum := amount + potentialFee + wallet.MinUTxODefaultValue
	testToken := "353436353733373437343646364236353645"

	inputs, err := wallet.GetUTXOsForAmount(
		ctx,
		txProvider,
		cardanoWalletAddr,
		[]string{wallet.AdaTokenName, fmt.Sprintf("%s.%s", policyID, testToken)},
		map[string]uint64{
			wallet.AdaTokenName:                       desiredSum,
			fmt.Sprintf("%s.%s", policyID, testToken): uint64(10)},
		map[string]uint64{
			wallet.AdaTokenName:                       desiredSum,
			fmt.Sprintf("%s.%s", policyID, testToken): uint64(10)})
	if err != nil {
		return "", err
	}

	fmt.Printf("DN_LOG_TAG INPUTS: %+v\n", inputs.Sum)
	fmt.Printf("DN_LOG_TAG INPUTS: %s %s %+v\n", policyID, testToken, inputs.Sum[fmt.Sprintf("%s.%s", policyID, testToken)])

	rawTx, txHash, err := CreateTxNativeTokens(
		cardanoCliBinary,
		networkTestMagic, protocolParams,
		qtd.Slot+ttlSlotNumberInc, metadata,
		outputs, inputs, cardanoWalletAddr, policyID)
	if err != nil {
		fmt.Printf("DN_LOG_TAG CreateTxNativeTokens error")
		return "", err
	}

	witness, err := wallet.CreateTxWitness(txHash, cardanoWallet)
	if err != nil {
		fmt.Printf("DN_LOG_TAG CreateTxWitness error")
		return "", err
	}

	signedTx, err := AssembleTxWitnesses(cardanoCliBinary, rawTx, [][]byte{witness})
	if err != nil {
		fmt.Printf("DN_LOG_TAG AssembleTxWitnesses error")
		return "", err
	}

	return txHash, txProvider.SubmitTx(ctx, signedTx)
}

func GetGenesisWalletFromCluster(
	dirPath string,
	keyID uint,
) (*wallet.Wallet, error) {
	keyFileName := strings.Join([]string{"utxo", fmt.Sprint(keyID)}, "")

	sKey, err := wallet.NewKey(filepath.Join(dirPath, "utxo-keys", fmt.Sprintf("%s.skey", keyFileName)))
	if err != nil {
		return nil, err
	}

	sKeyBytes, err := sKey.GetKeyBytes()
	if err != nil {
		return nil, err
	}

	vKey, err := wallet.NewKey(filepath.Join(dirPath, "utxo-keys", fmt.Sprintf("%s.vkey", keyFileName)))
	if err != nil {
		return nil, err
	}

	vKeyBytes, err := vKey.GetKeyBytes()
	if err != nil {
		return nil, err
	}

	return wallet.NewWallet(vKeyBytes, sKeyBytes), nil
}

/*
// CreateTx creates tx and returns cbor of raw transaction data, tx hash and error
func CreateTx(
	cardanoCliBinary string,
	testNetMagic uint,
	protocolParams []byte,
	timeToLive uint64,
	metadataBytes []byte,
	outputs []wallet.TxOutput,
	inputs wallet.TxInputs,
	changeAddress string,
) ([]byte, string, error) {
	outputsSum := wallet.GetOutputsSum(outputs)[wallet.AdaTokenName]

	builder, err := wallet.NewTxBuilder(cardanoCliBinary)
	if err != nil {
		return nil, "", err
	}

	defer builder.Dispose()

	if len(metadataBytes) != 0 {
		builder.SetMetaData(metadataBytes)
	}

	builder.SetProtocolParameters(protocolParams).SetTimeToLive(timeToLive).
		SetTestNetMagic(testNetMagic).
		AddInputs(inputs.Inputs...).
		AddOutputs(outputs...).AddOutputs(wallet.TxOutput{Addr: changeAddress})

	fee, err := builder.CalculateFee(0)
	if err != nil {
		return nil, "", err
	}

	// DN_TODO: should probably include check for native tokens in the change. Might fail when testing both directions
	change := inputs.Sum[wallet.AdaTokenName] - outputsSum - fee
	// handle overflow or insufficient amount
	if change > inputs.Sum[wallet.AdaTokenName] || (change > 0 && change < wallet.MinUTxODefaultValue) {
		return []byte{}, "", fmt.Errorf("insufficient amount %+v for %d or min utxo not satisfied",
			inputs.Sum, outputsSum+fee)
	}

	if change == 0 {
		builder.RemoveOutput(-1)
	} else {
		builder.UpdateOutputAmount(-1, change)
	}

	builder.SetFee(fee)

	return builder.Build()
}
*/
// CreateTx creates tx and returns cbor of raw transaction data, tx hash and error
func CreateTx(
	cardanoCliBinary string,
	testNetMagic uint,
	protocolParams []byte,
	timeToLive uint64,
	metadataBytes []byte,
	outputs []wallet.TxOutput,
	inputs wallet.TxInputs,
	changeAddress string,
) ([]byte, string, error) {
	outputsSum := wallet.GetOutputsSum(outputs)[wallet.AdaTokenName]

	lovelaceInputAmount := inputs.Sum[wallet.AdaTokenName]

	builder, err := wallet.NewTxBuilder(cardanoCliBinary)
	if err != nil {
		return nil, "", err
	}

	defer builder.Dispose()

	if len(metadataBytes) != 0 {
		builder.SetMetaData(metadataBytes)
	}

	builder.SetProtocolParameters(protocolParams).SetTimeToLive(timeToLive).
		SetTestNetMagic(testNetMagic).
		AddInputs(inputs.Inputs...)
	// AddOutputs(outputs...).AddOutputs(wallet.TxOutput{Addr: changeAddress})

	remainingTokens, err := wallet.GetTokensFromSumMap(inputs.Sum, wallet.AdaTokenName)
	if err != nil {
		return nil, "", err
	}

	fmt.Printf("DN_LOG_TAG outputs %+v\n", outputs)

	builder.AddOutputs(outputs...).
		AddOutputs(wallet.TxOutput{
			Addr:   changeAddress,
			Tokens: remainingTokens,
		})

	fee, err := builder.CalculateFee(0)
	if err != nil {
		return nil, "", err
	}

	change := lovelaceInputAmount - outputsSum - fee
	if change > lovelaceInputAmount || (change > 0 && change < wallet.MinUTxODefaultValue) {
		return []byte{}, "", fmt.Errorf("insufficient amount %+v for %d or min utxo not satisfied",
			inputs.Sum, outputsSum+fee)
	}

	if change == 0 {
		builder.RemoveOutput(-1)
	} else {
		builder.UpdateOutputAmount(-1, change)
	}

	builder.SetFee(fee)

	return builder.Build()
}

func CreateTxNativeTokens(
	cardanoCliBinary string,
	testNetMagic uint,
	protocolParams []byte,
	timeToLive uint64,
	metadataBytes []byte,
	outputs []wallet.TxOutput,
	inputs wallet.TxInputs,
	changeAddress string,
	policyID string,
) ([]byte, string, error) {
	outputsSum := wallet.GetOutputsSum(outputs)

	builder, err := wallet.NewTxBuilder(cardanoCliBinary)
	if err != nil {
		return nil, "", err
	}

	defer builder.Dispose()

	if len(metadataBytes) != 0 {
		builder.SetMetaData(metadataBytes)
	}

	builder.SetProtocolParameters(protocolParams).SetTimeToLive(timeToLive).
		SetTestNetMagic(testNetMagic).
		AddInputs(inputs.Inputs...).
		AddOutputs(outputs...)

	lovelaceInputAmount := inputs.Sum[wallet.AdaTokenName]

	testToken := "353436353733373437343646364236353645"
	testTokenName := fmt.Sprintf("%s.%s", policyID, testToken)

	remainingTokens, _ := wallet.GetTokensFromSumMap(inputs.Sum, wallet.AdaTokenName)

	ntOutputsSum := uint64(0)
	for i, inputToken := range remainingTokens {
		if "54657374746F6B656E" == inputToken.Name {
			ntOutputsSum = remainingTokens[i].Amount - uint64(10)
			remainingTokens[i].Amount -= uint64(10)

			break
		}
	}

	ntChange := inputs.Sum[testTokenName] - uint64(10)
	fmt.Printf("DN_LOG_TAG inputs.NativeTokenSum: %d, ntOutputsSum: %d\n", inputs.Sum[testTokenName], ntOutputsSum)

	builder.AddOutputs(wallet.TxOutput{
		Amount: wallet.MinUTxODefaultValue,
		Addr:   changeAddress,
		Tokens: remainingTokens,
	})

	fee, err := builder.CalculateFee(0)
	if err != nil {
		return nil, "", err
	}

	change := lovelaceInputAmount - outputsSum[wallet.AdaTokenName] - fee

	// handle overflow or insufficient amount
	if change > inputs.Sum[wallet.AdaTokenName] || (change > 0 && change < wallet.MinUTxODefaultValue) {
		return []byte{}, "", fmt.Errorf("insufficient amount %d for %d or min utxo not satisfied",
			inputs.Sum[wallet.AdaTokenName], outputsSum[wallet.AdaTokenName]+fee)
	}

	if ntChange > inputs.Sum[testTokenName] {
		return []byte{}, "", fmt.Errorf("insufficient amount %d for native tokens amount %d",
			inputs.Sum[testTokenName], ntOutputsSum)
	}

	if change == 0 && ntChange == 0 {
		fmt.Printf("DN_LOG_TAG remove change: %d, ntChange: %d\n", change, ntChange)
		builder.RemoveOutput(-1)
	} else {
		fmt.Printf("DN_LOG_TAG change: %d, ntChange: %d\n", change, ntChange)
		builder.UpdateOutputAmount(-1, change)
	}

	builder.SetFee(fee)

	fmt.Printf("FEE %d\n", fee)

	return builder.Build()
}

// CreateTxWitness creates cbor of vkey+signature pair of tx hash
func CreateTxWitness(txHash string, key wallet.ITxSigner) ([]byte, error) {
	return wallet.CreateTxWitness(txHash, key)
}

// AssembleTxWitnesses assembles all witnesses in final cbor of signed tx
func AssembleTxWitnesses(cardanoCliBinary string, txRaw []byte, witnesses [][]byte) ([]byte, error) {
	builder, err := wallet.NewTxBuilder(cardanoCliBinary)
	if err != nil {
		return nil, err
	}

	defer builder.Dispose()

	return builder.AssembleTxWitnesses(txRaw, witnesses)
}

// TODO: Remove? Fix output per token name and policy
func GetNativeTokenOutputsSum(outputs []wallet.TxOutput) (receiversSum uint64) {
	for _, x := range outputs {
		for _, t := range x.Tokens {
			receiversSum += t.Amount
		}
	}
	return receiversSum
}

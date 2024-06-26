package bot

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/webhook"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
	"github.com/kallydev/pancakeswap-prediction-bot/contract/pancakeswap"
	"github.com/robfig/cron"
	"github.com/sirupsen/logrus"
)

type Bot struct {
	ctx                context.Context
	config             *Config
	chronos            *cron.Cron
	ethereumClient     *ethclient.Client
	predictionContract *pancakeswap.Prediction
	chainID            *big.Int
	address            common.Address
	privateKey         *ecdsa.PrivateKey
}

var currentround *big.Int = big.NewInt(0)

func (b *Bot) Run() error {
	var first = true
	for {

		if first {
			first = false
		} else {
			time.Sleep(time.Second * 30)
		}
		paused, err := b.predictionContract.Paused(&bind.CallOpts{})
		if err != nil {
			logrus.Errorln(err)
			continue
		}
		if paused {
			continue
		}
		currentEpoch, err := b.predictionContract.CurrentEpoch(&bind.CallOpts{})
		if err != nil {
			logrus.Errorln(err)
			continue
		}
		logrus.Infof("Current epoch %d\n", currentEpoch)
		round, err := b.predictionContract.Round(&bind.CallOpts{}, currentEpoch)
		minusOne := new(big.Int).SetInt64(1)
		result := new(big.Int).Sub(currentEpoch, minusOne)
		nigga := currentround.Cmp(result)
		logrus.Infof("Current Betted Round %d,%d\n", result, nigga)
		if nigga == 0 {
			logrus.Infof("test")
			go func() {
				for {
					round, err := b.predictionContract.Round(&bind.CallOpts{}, currentround)
					if err != nil {
						logrus.Errorln(err)
						return
					}
					var url = "https://discord.com/api/webhooks/1215039636355940422/x57nhkbe2JCZ4FmM5LjEZebr4a5gCARsnOUQDWl1xGj10-CzOmJnG9_ejTv1dQk9M0rr"
					client, err := webhook.NewWithURL(url)
					if err != nil {
						logrus.Errorln(err)
					}
					bear, _ := new(big.Float).SetInt(round.BearAmount).Float64()
					bull, _ := new(big.Float).SetInt(round.BearAmount).Float64()
					lockprice, _ := new(big.Float).SetInt(round.LockPrice).Float64()
					_, err = client.CreateMessage(discord.WebhookMessageCreate{Content: fmt.Sprintf("Round Information:\nLock Price: %d\nBull Amount: %f\nBear Amount: %f\n?: %d", lockprice/params.Ether, bull/params.Ether, bear/params.Ether, round.LockTimestamp)})
					if err != nil {
						logrus.Errorln(err)
					}

					//logrus.Infof("Round Information:\nLock Price: %d\nBull Amount: %d\nBear Amount: %d\n?: %d", round.LockPrice, round.BullAmount / params.Ether, round.BearAmount, round.LockTimestamp)
					time.Sleep(time.Second * 5)
				}
			}()
		}
		if err != nil {
			logrus.Errorln(err)
			continue
		}
		lockTime := time.Unix(round.LockTimestamp.Int64(), 0)
		if round.OracleCalled || time.Now().After(lockTime) {
			continue
		}
		timer := time.NewTimer(lockTime.Sub(time.Now().Add(time.Second * time.Duration(b.config.Time))))
		select {
		case <-timer.C:
			round, err = b.predictionContract.Round(&bind.CallOpts{}, currentEpoch)
			if err != nil {
				logrus.Errorln(err)
				continue
			}
			if round.OracleCalled || time.Now().After(lockTime) {
				continue
			}
			var transaction *types.Transaction
			//amount := big.NewInt(int64(b.config.Amount * params.Ether))
			if round.BullAmount.Cmp(round.BearAmount) == 1 {
				logrus.Infof("Bet bull %f BNB\n", b.config.Amount)
				currentround = currentEpoch
				//transaction, err = b.predictionContract.BetBull(b.newTransactOption(amount), currentEpoch)
				/*if err != nil {
					logrus.Errorln(err)
					continue
				}*/
			} else {
				logrus.Infof("Bet bear %f BNB\n", b.config.Amount)
				currentround = currentEpoch
				//transaction, err = b.predictionContract.BetBear(b.newTransactOption(amount), currentEpoch)
				/*if err != nil {
					logrus.Errorln(err)
					continue
				}*/
			}
			if transaction == nil {
				continue
			}
			logrus.Infof("Transaction detail https://bscscan.com/tx/%s\n", transaction.Hash().Hex())
		}
	}
}

func (b *Bot) newTransactOption(value *big.Int) *bind.TransactOpts {
	return &bind.TransactOpts{
		From:  b.address,
		Value: value,
		Signer: func(address common.Address, transaction *types.Transaction) (*types.Transaction, error) {
			return types.SignTx(transaction, types.LatestSignerForChainID(b.chainID), b.privateKey)
		},
	}
}

func (b *Bot) initializeEthereumClient() error {
	ethereumClient, err := ethclient.Dial(b.config.Endpoint)
	if err != nil {
		return err
	}
	b.ethereumClient = ethereumClient
	logrus.Infoln("Connected to Ethereum endpoint provider")
	chainID, err := b.ethereumClient.ChainID(b.ctx)
	if err != nil {
		return err
	}
	b.chainID = chainID
	return nil
}

func (b *Bot) initializePredictionContract() error {
	predictionContract, err := pancakeswap.NewPrediction(pancakeswap.AddressPrediction, b.ethereumClient)
	if err != nil {
		return err
	}
	b.predictionContract = predictionContract
	return nil
}

type Config struct {
	Endpoint      string
	PrivateKeyHex string
	Amount        float64
	Time          int
}

func New(ctx context.Context, config *Config) (*Bot, error) {
	if config == nil {
		return nil, errors.New("config can't is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	privateKey, err := crypto.HexToECDSA(strings.TrimLeft(config.PrivateKeyHex, "0x"))
	if err != nil {
		return nil, err
	}
	bot := &Bot{
		ctx:        ctx,
		chronos:    cron.New(),
		config:     config,
		privateKey: privateKey,
		address:    crypto.PubkeyToAddress(*privateKey.Public().(*ecdsa.PublicKey)),
	}
	if err := bot.initializeEthereumClient(); err != nil {
		return nil, err
	}
	if err := bot.initializePredictionContract(); err != nil {
		return nil, err
	}
	logrus.Infof("Wallet address https://bscscan.com/address/%s", bot.address.Hex())
	return bot, nil
}

package main

import (
	"fmt"
	"sync"

	"net"

	"io"

	"bufio"

	"strconv"

	"log"

	"time"

	"encoding/json"

	"crypto/rand"

	"math/big"

	"os"

	"github.com/Eetin/blockchain"
	"github.com/davecgh/go-spew/spew"
	"github.com/joho/godotenv"
)

// Blockchain is a series of validated blocks
var Blockchain []blockchain.POSBlock
var tempBlocks []blockchain.POSBlock

// candidate blocks handles incoming blocks for validation
var candidateBlocks = make(chan blockchain.POSBlock)

// announcements broadcasts winning validator to all nodes
var announcements = make(chan string)

var mutex = &sync.Mutex{}

// validators keep track of open validators and balances
var validators = make(map[string]int)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	// create genesis block
	t := time.Now()
	genesisBlock := blockchain.POSBlock{
		Index:     0,
		Timestamp: t.String(),
		BPM:       0,
		PrevHash:  "",
		Validator: "",
	}
	genesisBlock.Hash = blockchain.CalculatePOSBlockHash(genesisBlock)
	spew.Dump(genesisBlock)
	Blockchain = append(Blockchain, genesisBlock)

	// start TCP server
	server, err := net.Listen("tcp", ":"+os.Getenv("ADDR"))
	if err != nil {
		log.Fatal(err)
	}
	defer server.Close()

	go func() {
		for candidate := range candidateBlocks {
			mutex.Lock()
			tempBlocks = append(tempBlocks, candidate)
			mutex.Unlock()
		}
	}()

	go func() {
		for {
			pickWinner()
		}
	}()

	for {
		conn, err := server.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()

	go func() {
		for {
			msg := <-announcements
			_, err := io.WriteString(conn, msg)
			if err != nil {
				conn.Close()
				log.Printf("cannot send announcement: %v", err)
				return
			}
		}
	}()

	// validator address
	var address string

	// allow user to allocate number of tokens to stake
	// the greater the number of tokens, the greater chance to forge a new block
	_, err := io.WriteString(conn, "Enter token balance: ")
	if err != nil {
		log.Printf("failed to ask for token balance: %v", err)
		conn.Close()
		return
	}
	scanBalance := bufio.NewScanner(conn)
	if ok := scanBalance.Scan(); !ok {
		log.Printf("balance fetching failed: %v", err)
		return
	}

	balance, err := strconv.Atoi(scanBalance.Text())
	if err != nil {
		log.Printf("%v not a number: %v", scanBalance.Text(), err)
		return
	}

	t := time.Now()
	address = blockchain.CalculateHash(t.String())
	mutex.Lock()
	validators[address] = balance
	mutex.Unlock()
	log.Println(validators)

	_, err = io.WriteString(conn, "\nEnter BPM: ")
	if err != nil {
		log.Printf("failed to ask for BMP", err)
		return
	}

	scanBPM := bufio.NewScanner(conn)

	go func() {
		for {
			// take in BPM from stdin and add it to blockchain after validating
			for scanBPM.Scan() {
				bpm, err := strconv.Atoi(scanBPM.Text())
				if err != nil {
					// if malicious party tries to mutate the chain with a bad input, delete them as a validator and they lose their staked tokens
					fmt.Printf("%v not a number: %v", scanBPM.Text(), err)
					mutex.Lock()
					delete(validators, address)
					mutex.Unlock()
					conn.Close()
					return
				}

				mutex.Lock()
				oldLastIndex := Blockchain[len(Blockchain)-1]
				mutex.Unlock()

				// create new block to consider forging
				newBlock, err := blockchain.GeneratePOSBlock(oldLastIndex, bpm, address)
				if err != nil {
					log.Printf("new POS block generation failed: %v", err)
					continue
				}
				if blockchain.IsPOSBlockValid(newBlock, oldLastIndex) {
					candidateBlocks <- newBlock
				}
				_, err = io.WriteString(conn, "Enter BPM: ")
				if err != nil {
					log.Printf("failed to ask for BMP", err)
					conn.Close()
					return
				}
			}
		}
	}()

	// broadcast Blockchain
	for {
		time.Sleep(time.Minute)
		mutex.Lock()
		output, err := json.Marshal(Blockchain)
		mutex.Unlock()
		if err != nil {
			log.Fatalf("cannot marshal Blockchain to JSON", err)
		}
		_, err = io.WriteString(conn, string(output)+"\n")
		if err != nil {
			log.Printf("cannot send Blockchain: %v", err)
			return
		}
	}
}

// pickWinner creates a lottery pool of validators and chooses the validator who gets to forge a block to the blockchain
// by random selecting from the pool, weighted by amount of tokens staked
func pickWinner() {
	time.Sleep(30 * time.Second)
	mutex.Lock()
	temp := tempBlocks
	mutex.Unlock()

	var lotteryPool []string
	if len(temp) > 0 {
		// slightly modified traditional proof of stake algorithm
		// from all validators who submitted a block, weight them by the number of staked tokens
		// in traditional proof of stake, validators can participate without submitting a block to be forged
	OUTER:
		for _, block := range temp {
			// skip if already in the lottery pool
			for _, node := range lotteryPool {
				if block.Validator == node {
					continue OUTER
				}
			}

			mutex.Lock()
			setValidators := validators
			mutex.Unlock()

			k, ok := setValidators[block.Validator]
			if !ok {
				continue
			}
			mutex.Lock()
			for i := 0; i < k; i++ {
				lotteryPool = append(lotteryPool, block.Validator)
			}
			mutex.Unlock()
		}

		// randomly pick the winner from the lottery pool
		sBig, err := rand.Int(rand.Reader, big.NewInt(int64(len(lotteryPool))))
		if err != nil {
			log.Printf("failed to generate random number", err)
			return
		}
		s := sBig.Int64()
		lotteryWinner := lotteryPool[s]

		// add winner's block to Blockchain and let all the other nodes know
		for _, block := range temp {
			if block.Validator == lotteryWinner {
				mutex.Lock()
				Blockchain = append(Blockchain, block)
				mutex.Unlock()
				for _ = range validators {
					announcements <- "\nwinning validator: " + lotteryWinner + "\n"
				}
				break
			}
		}
	}
	mutex.Lock()
	tempBlocks = []blockchain.POSBlock{}
	mutex.Unlock()
}

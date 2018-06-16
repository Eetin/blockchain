package main

import (
	"io"
	"log"
	"net"

	"time"

	"os"

	"bufio"

	"strconv"

	"encoding/json"

	"github.com/Eetin/blockchain"
	"github.com/davecgh/go-spew/spew"
	"github.com/joho/godotenv"
)

var Blockchain []blockchain.Block

var bcServer chan []blockchain.Block

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	bcServer = make(chan []blockchain.Block)

	// create genesis block
	t := time.Now()
	genesisBlock := blockchain.Block{
		Index:     0,
		Timestamp: t.String(),
		BPM:       0,
		Hash:      "",
		PrevHash:  "",
	}
	spew.Dump(genesisBlock)
	Blockchain = append(Blockchain, genesisBlock)

	// start TCP server
	server, err := net.Listen("tcp", ":"+os.Getenv("ADDR"))
	if err != nil {
		log.Fatal(err)
	}
	defer server.Close()

	for {
		conn, err := server.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handleConn(conn)
	}
}

func replaceChain(newBlocks []blockchain.Block) {
	if len(newBlocks) > len(Blockchain) {
		Blockchain = newBlocks
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()

	io.WriteString(conn, "Enter BPM: ")

	scanner := bufio.NewScanner(conn)

	// take BPM from stdin and add it to the blockchain after validating
	go func() {
		for scanner.Scan() {
			bpm, err := strconv.Atoi(scanner.Text())
			if err != nil {
				log.Printf("%v not a number: %v", scanner.Text(), err)
				continue
			}
			newBlock, err := blockchain.GenerateBlock(Blockchain[len(Blockchain)-1], bpm)
			if err != nil {
				log.Println(err)
				continue
			}
			if !blockchain.IsBlockValid(newBlock, Blockchain[len(Blockchain)-1]) {
				log.Println("invalid block")
				continue
			}
			newBlockchain := append(Blockchain, newBlock)
			replaceChain(newBlockchain)

			bcServer <- Blockchain
			io.WriteString(conn, "\nEnter BPM: ")
		}
	}()

	// simulate reveiving broadcasts
	go func() {
		for {
			time.Sleep(30 * time.Second)
			output, err := json.Marshal(Blockchain)
			if err != nil {
				log.Fatal(err)
			}
			io.WriteString(conn, string(output))
		}
	}()

	for _ = range bcServer {
		spew.Dump(Blockchain)
	}
}

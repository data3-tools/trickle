package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/dereference-xyz/trickle/decode"
	"github.com/dereference-xyz/trickle/load"
	"github.com/dereference-xyz/trickle/model"
	"github.com/dereference-xyz/trickle/node"
	"github.com/dereference-xyz/trickle/service"
	"github.com/dereference-xyz/trickle/store"
	"github.com/gagliardetto/solana-go/rpc"
	"gorm.io/driver/sqlite"
)

func main() {
	idlJsonFile := flag.String("idl", "", "path to idl.json file")
	programId := flag.String("programId", "", "program id to pull data for")
	flag.Parse()

	decoderFilePath := "js/decoder/anchor/dist/decoder.js"
	decoderCode, err := os.ReadFile(decoderFilePath)
	if err != nil {
		panic(err)
	}

	idlJson, err := os.ReadFile(*idlJsonFile)
	if err != nil {
		panic(err)
	}

	programType, err := model.FromIDL(idlJson)
	if err != nil {
		panic(err)
	}

	// TODO: Add CLI flag for db path.
	accountStore, err := store.NewAccountStore(sqlite.Open("./test.db"))
	err = accountStore.AutoMigrate(programType)
	if err != nil {
		panic(err)
	}

	solanaNode := node.NewSolanaNode(rpc.MainNetBeta_RPC)
	decodeEngine := decode.NewV8Engine()
	loader := load.NewLoader(solanaNode, decodeEngine, accountStore)
	decoder := decode.NewAnchorAccountDecoder(string(decoderCode), string(idlJson), decoderFilePath)

	err = loader.Load(decoder, *programId)
	if err != nil {
		panic(err)
	}

	fmt.Println("Data loaded successfully.")
	fmt.Println("Running service...")

	srv := service.NewService(accountStore, programType)
	err = srv.Router().Run()
	if err != nil {
		panic(err)
	}
}

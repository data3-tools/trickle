package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/dereference-xyz/trickle/config"
	"github.com/dereference-xyz/trickle/decode"
	"github.com/dereference-xyz/trickle/load"
	"github.com/dereference-xyz/trickle/mocks"
	"github.com/dereference-xyz/trickle/model"
	"github.com/dereference-xyz/trickle/store"
	"github.com/dereference-xyz/trickle/store/sqlite"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dummyProgramId = "0xdeadbeef"
const testIdlFile = "../test/squads_mpl.json"
const testGetProgramAccountsFile = "../test/squads_mpl_accounts.json"

type Deps struct {
	ctrl         *gomock.Controller
	programType  *model.ProgramType
	solanaNode   *mocks.MockSolanaNode
	accountStore *store.AccountStore
	service      *Service
	router       *gin.Engine
	loader       *load.Loader
	decoder      decode.Decoder
}

func (deps *Deps) Finish() {
	deps.ctrl.Finish()
}

func loadTestIDL(t *testing.T) ([]byte, *model.ProgramType) {
	idlJson, err := os.ReadFile(testIdlFile)
	require.NoError(t, err)
	programType, err := model.FromIDL(idlJson)
	require.NoError(t, err)
	return idlJson, programType
}

func initDeps(t *testing.T, deps *Deps) *Deps {
	deps.ctrl = gomock.NewController(t)

	idlJson, programType := loadTestIDL(t)
	if deps.programType == nil {
		deps.programType = programType
	}

	if deps.solanaNode == nil {
		deps.solanaNode = mocks.NewMockSolanaNode(deps.ctrl)
	}

	if deps.accountStore == nil {
		accountStore, err := store.NewAccountStore(sqlite.NewDriver(":memory:"))
		require.NoError(t, err)
		require.NoError(t, accountStore.AutoMigrate(deps.programType))
		deps.accountStore = accountStore
	}

	if deps.service == nil {
		deps.service = NewService(deps.accountStore, deps.programType)
	}

	if deps.router == nil {
		deps.router = deps.service.Router()
	}

	if deps.loader == nil {
		decodeEngine := decode.NewV8Engine()
		deps.loader = load.NewLoader(deps.solanaNode, decodeEngine, deps.accountStore)
	}

	if deps.decoder == nil {
		decoder, err := decode.NewAnchorAccountDecoder("../"+config.DecoderFilePath, string(idlJson))
		require.NoError(t, err)
		deps.decoder = decoder
	}

	return deps
}

func parseGetProgramAccountsResult(t *testing.T, jsonStr string) rpc.GetProgramAccountsResult {
	var result rpc.GetProgramAccountsResult
	err := json.Unmarshal([]byte(jsonStr), &result)
	require.NoError(t, err)
	return result
}

func serveRequest(t *testing.T, router *gin.Engine, method, url, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	reader := strings.NewReader(body)
	req, err := http.NewRequest(method, url, reader)
	require.NoError(t, err)
	router.ServeHTTP(recorder, req)
	return recorder
}

func getRequest(t *testing.T, router *gin.Engine, path string, params map[string]interface{}) *httptest.ResponseRecorder {
	url := &url.URL{
		Path: "/api" + path,
	}
	q := url.Query()
	for k, v := range params {
		q.Set(k, fmt.Sprintf("%v", v))
	}
	url.RawQuery = q.Encode()
	return serveRequest(t, router, "GET", url.String(), "")
}

func v1SolanaAccountRead(t *testing.T, router *gin.Engine, accountType string, predicates map[string]interface{}) *httptest.ResponseRecorder {
	return getRequest(t, router, fmt.Sprintf("/v1/solana/account/read/%s", accountType), predicates)
}

func TestBooleanSQLiteSerialization(t *testing.T) {
	deps := initDeps(t, &Deps{})
	deps.Finish()

	deps.solanaNode.EXPECT().GetProgramAccounts(dummyProgramId).Return(parseGetProgramAccountsResult(t, `
	[
		{
			"pubkey":"AMg3hcqcNnuuLTF7ui3iBFtBX9KJrP96SnRXkPr8CXUT",
			"account":{
				"lamports":2707440,
				"owner":"SMPLecH534NA9acpos4G6x7uf3LWbCAwZQE9e8ZekMu",
				"data":[
					"7rl+lb1Z/1wGfpHdznHAx18//7oe1iyvQARvHdjDUog7EKh9tzrZMAUAAAAFEDWzb8ZQG2wusfepjC079X0ogkZUTrX9lN0P0sYqpAABBRA1s2/GUBtsLrH3qYwtO/V9KIJGVE61/ZTdD9LGKqQBAcCiNCRvzRzPNYMafIhBLBRvyR5hZH2aQMXaOwoJMUm2AQEGp9UXGSxcUSGMyUw9SvF/WNruCJuh/UTj29mKAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACgAAAANdHuCfsY5IucFvY3L4K/GkofeZWEMwteLWwiE+IC8lnd8Ck5flvybAf4A",
					"base64"
				],
				"executable":false,
				"rentEpoch":337
			}
		}
	]
	`), nil)
	require.NoError(t, deps.loader.Load(deps.decoder, dummyProgramId))

	recorder := v1SolanaAccountRead(t, deps.router, "MsInstruction", map[string]interface{}{})
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `
	{
		"accounts": []
	}
	`, recorder.Body.String())
}
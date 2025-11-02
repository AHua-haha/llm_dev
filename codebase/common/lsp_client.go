package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	_ "llm_dev/utils"
	"net/http"
	"os/exec"
	"time"

	"github.com/rs/zerolog/log"
)

var G_lspClient *lspClient

func InitLsp() {
	template := `
source /root/workspace/llm_dev/.venv/bin/activate
python /root/workspace/llm_dev/multilspy_client.py %d
	`
	port := 5000
	command := fmt.Sprintf(template, port)
	cmd := exec.Command("bash", "-c", command)
	// err := cmd.Start()
	// if err != nil {
	// 	log.Fatal().Err(err).Msg("start multilspy server failed")
	// 	return
	// }
	G_lspClient = &lspClient{
		httpClient: &http.Client{},
		bashUrl:    fmt.Sprintf("http://127.0.0.1:%d", port),
		serverCmd:  cmd,
	}
	time.Sleep(1 * time.Second)
	args := map[string]any{
		"lang": "go",
		"root": "/root/workspace/llm_dev",
	}
	resp, err := G_lspClient.sendReq("POST", "/setup", args)
	if err != nil {
		log.Fatal().Err(err).Msg("start multilspy server failed")
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal().Err(err).Msg("start multilspy server failed")
	}
	res := struct {
		Code int
	}{
		Code: -1,
	}
	err = json.Unmarshal(data, &res)
	if err != nil || res.Code != 0 {
		log.Fatal().Err(err).Msg("start multilspy server failed")
	}
	log.Info().Msg("init lsp server and client")
}

func CloseLsp() {
	if G_lspClient == nil {
		return
	}
	// err := g_lspClient.serverCmd.Process.Kill()
	// if err != nil {
	// 	return
	// }
}

type lspClient struct {
	httpClient *http.Client
	bashUrl    string
	serverCmd  *exec.Cmd
}

type Point struct {
	Line   uint `json:"line"`
	Column uint `json:"column"`
}

type RequestDefinitionArgs struct {
	File string  `json:"file"`
	Loc  []Point `json:"loc"`
}

func (lsp *lspClient) sendReq(method string, route string, data any) (*http.Response, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	url := lsp.bashUrl + route
	req, err := http.NewRequest(method, url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	if err != nil {
		return nil, err
	}

	return lsp.httpClient.Do(req)
}

func (lsp *lspClient) RequestDefinition(args RequestDefinitionArgs) {
	resp, err := lsp.sendReq("GET", "/definition", args)
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	fmt.Printf("%v\n", string(data))
}

func (lsp *lspClient) requestReference(args RequestDefinitionArgs) {
	resp, err := lsp.sendReq("GET", "/reference", args)
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	var res any
	json.Unmarshal(data, &res)
	prettyJson, _ := json.MarshalIndent(res, "", "  ")
	fmt.Print(string(prettyJson))
	fmt.Print("\n")
}

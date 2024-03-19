package tmt2

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/GSH-LAN/Unwindia_common/src/go/config"
	"github.com/GSH-LAN/Unwindia_common/src/go/matchservice"
	"github.com/GSH-LAN/Unwindia_common/src/go/template"
	tmt2_go "github.com/GSH-LAN/Unwindia_tmt2/pkg/tmt2-go"
	"log/slog"
	"net/http"
	"time"
)

const jsonContentType = "application/json"

type TMT2ClientImpl struct {
	tmt2Client        tmt2_go.ClientWithResponsesInterface
	config            config.ConfigClient
	matchTemplateName string
}

type enrichedMatchInfo struct {
	MatchInfo *matchservice.MatchInfo
	host      string
	port      string
}

func NewTMT2Client(configClient config.ConfigClient, url, adminToken, matchTemplateName string) (*TMT2ClientImpl, error) {
	httpClient := &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			MaxIdleConns:          20,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       600 * time.Second,
			TLSHandshakeTimeout:   30 * time.Second,
			ExpectContinueTimeout: 30 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		},
	}

	authorizedRequestEditor := func(ctx context.Context, req *http.Request) error {
		req.Header.Add("Authorization", adminToken)
		return nil
	}

	tmt2Client, err := tmt2_go.NewClientWithResponses(
		url,
		tmt2_go.WithHTTPClient(httpClient),
		tmt2_go.WithRequestEditorFn(authorizedRequestEditor),
	)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	response, err := tmt2Client.Login(ctx)
	if err != nil {
		return nil, err
	}

	slog.Debug("tmt2Client Login respone", "response", *response)

	matchTemplateName = fmt.Sprintf("%s.gohtml", matchTemplateName)
	// verify template exists
	_, ok := configClient.GetConfig().Templates[matchTemplateName]
	if !ok {
		return nil, fmt.Errorf("template %s not found", matchTemplateName)
	}

	return &TMT2ClientImpl{
		config:            configClient,
		tmt2Client:        tmt2Client,
		matchTemplateName: matchTemplateName,
	}, nil
}

func (t *TMT2ClientImpl) CreateMatch(ctx context.Context, matchInfo *matchservice.MatchInfo) (*tmt2_go.CreateMatchResponse, error) {

	parsedTmt2MatchTemplate, err := template.ParseTemplateForMatch(t.config.GetConfig().Templates[t.matchTemplateName], matchInfo)
	if err != nil {
		slog.Error("Error parsing template", "err", err)
		return nil, err
	}

	//bytes, err := json.Marshal(parsedTmt2MatchTemplate)
	//if err != nil {
	//	return nil, err
	//}
	//
	//tmt2RequestBody := tmt2_go.CreateMatchJSONRequestBody{}
	//err = json.Unmarshal(bytes, &tmt2RequestBody)
	//if err != nil {
	//	return nil, err
	//}

	buf := []byte(parsedTmt2MatchTemplate)
	bodyReader := bytes.NewReader(buf)
	//response, err := t.tmt2Client.CreateMatch(ctx, tmt2RequestBody)
	createMatchResponse, err := t.tmt2Client.CreateMatchWithBodyWithResponse(ctx, jsonContentType, bodyReader)
	if err != nil {
		return nil, err
	}
	//
	//createMatchResponse := tmt2_go.CreateMatchResponse{}
	//err = json.NewDecoder(response.Body).Decode(&createMatchResponse)
	//if err != nil {
	//	return nil, err
	//}

	return createMatchResponse, nil
}

// GetMatch returns current match state from TMT2
func (t *TMT2ClientImpl) GetMatch(ctx context.Context, matchID string) (*tmt2_go.GetMatchResponse, error) {
	return t.tmt2Client.GetMatchWithResponse(ctx, matchID)
}

// DeleteMatch deletes a match from TMT2
func (t *TMT2ClientImpl) DeleteMatch(ctx context.Context, matchID string) error {
	_, err := t.tmt2Client.DeleteMatchWithResponse(ctx, matchID)
	return err
}

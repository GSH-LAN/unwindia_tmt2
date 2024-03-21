package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/GSH-LAN/Unwindia_common/src/go/config"
	"github.com/GSH-LAN/Unwindia_common/src/go/matchservice"
	"github.com/GSH-LAN/Unwindia_common/src/go/messagebroker"
	"github.com/GSH-LAN/Unwindia_common/src/go/template"
	tmt2_go "github.com/GSH-LAN/Unwindia_tmt2/pkg/tmt2-go"
	"github.com/GSH-LAN/Unwindia_tmt2/src/database"
	"github.com/GSH-LAN/Unwindia_tmt2/src/environment"
	"github.com/GSH-LAN/Unwindia_tmt2/src/messagequeue"
	"github.com/GSH-LAN/Unwindia_tmt2/src/models"
	"github.com/GSH-LAN/Unwindia_tmt2/src/router"
	"github.com/GSH-LAN/Unwindia_tmt2/src/tmt2"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/gammazero/workerpool"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log/slog"
	"sync"
)

type Server struct {
	ctx            context.Context
	env            *environment.Environment
	config         config.ConfigClient
	dbClient       database.DatabaseClient
	workerpool     *workerpool.WorkerPool
	subscriber     *messagequeue.Subscriber
	matchPublisher message.Publisher
	messageChan    chan *messagebroker.Message
	lock           sync.Mutex
	router         *gin.Engine
	stop           chan struct{}
}

func NewServer(ctx context.Context, env *environment.Environment, cfgClient config.ConfigClient, matchPublisher message.Publisher, wp *workerpool.WorkerPool) (*Server, error) {
	messageChan := make(chan *messagebroker.Message)

	subscriber, err := messagequeue.NewSubscriber(ctx, env, messageChan)
	if err != nil {
		return nil, err
	}

	db, err := database.NewClient(ctx, env)
	if err != nil {
		return nil, err
	}

	tmt2Client, err := tmt2.NewTMT2Client(cfgClient, env.TMT2URL, env.TMT2AccessToken, env.TMT2MatchTemplateName)
	if err != nil {
		return nil, err
	}

	go func() {
		_ = NewWorker(ctx, wp, db, matchPublisher, cfgClient, env.PulsarBaseTopic, tmt2Client, env.MatchDeleteWaitTime).StartWorker(env.JobsProcessInterval)
	}()

	srv := Server{
		ctx:            ctx,
		env:            env,
		config:         cfgClient,
		dbClient:       db,
		workerpool:     wp,
		subscriber:     subscriber,
		messageChan:    messageChan,
		lock:           sync.Mutex{},
		stop:           make(chan struct{}),
		router:         router.DefaultRouter(),
		matchPublisher: matchPublisher,
	}
	return &srv, nil
}

func (s *Server) Start() error {
	s.setupRouter()
	go func() {
		_ = s.router.Run(fmt.Sprintf(":%d", s.env.HTTPPort))
	}()

	s.subscriber.StartConsumer()
	for {
		select {
		case <-s.stop:
			slog.Info("Stopping processing, server stopped")
			return nil
		case message := <-s.messageChan:
			s.workerpool.Submit(func() {
				s.messageHandler(message)
			})
		}
	}
}

func (s *Server) Stop() error {
	slog.Info("Stopping server")
	close(s.stop)
	return fmt.Errorf("server Stopped")
}

func (s *Server) messageHandler(message *messagebroker.Message) {
	slog.Info("Received message", "message", message)

	bytes, err := json.Marshal(message.Data)
	if err != nil {
		slog.Error("Error decoding match", "error", err)
		return

	}

	var match matchservice.MatchInfo
	err = json.Unmarshal(bytes, &match)
	if err != nil {
		slog.Error("Error decoding match", "error", err)
		return

	}

	slog.Info("Received match", "match", match)

	switch message.SubType {
	case messagebroker.UNWINDIA_MATCH_SERVER_READY.String():
		err = s.handleServerReadyMessage(&match)
	case messagebroker.UNWINDIA_MATCH_FINISHED.String():
		err = s.handleMatchFinishedMessage(&match)

	}

	if err != nil {
		slog.Error("Error processing message", "error", err, "message", *message)
	}
}

// handle server ready message
func (s *Server) handleServerReadyMessage(match *matchservice.MatchInfo) error {
	// create server for match
	slog.Info("Match server is ready, saving Match to db", "id", match.Id)
	matchId := match.Id
	if s.env.UseMatchServiceId {
		matchId = match.MsID
	}

	// check if we already have a match with this id
	_, err := s.dbClient.GetMatchByMatchID(s.ctx, matchId)
	if err == nil {
		slog.Info("Match already exists in db", "id", match.Id)
		return nil
	}

	dbMatch := database.Match{
		MatchInfo: *match,
		MatchID:   matchId,
		JobState:  models.JOB_STATE_NEW,
	}

	objectId, err := s.dbClient.CreateMatch(s.ctx, &dbMatch)
	if err != nil {
		// TODO: some retry stuff we need :(
		slog.Error("Error creating job for match", "error", err)
		return err
	}
	slog.Debug("Created db entry for match", "id", match.Id, "objectId", objectId)
	return nil
}

// handle match finished message
func (s *Server) handleMatchFinishedMessage(match *matchservice.MatchInfo) error {
	// update match entry to finished and set timestamp, so it gets removed after configured time
	slog.Info("Match is finished, updating db", "id", match.Id)
	matchId := match.Id
	if s.env.UseMatchServiceId {
		matchId = match.MsID
	}

	dbMatch, err := s.dbClient.GetMatchByMatchID(s.ctx, matchId)
	if err != nil {
		slog.Error("Error getting match from db", "error", err)
		return err
	}

	dbMatch.JobState = models.JOB_STATE_FINISHED
	_, err = s.dbClient.UpdateMatch(s.ctx, dbMatch)
	if err != nil {
		slog.Error("Error updating match in db", "error", err)
		return err
	}
	slog.Debug("Updated db entry for match", "id", match.Id)
	return nil
}

func (s *Server) setupRouter() {
	internal := s.router.Group("/api/internal")
	internal.GET("/metrics", gin.WrapH(promhttp.Handler()))

	v1Api := s.router.Group("/api/v1")
	v1Api.POST("/webhook/:id", s.webhookHandler)
	v1Api.POST("/test_template", s.testTemplateHandler)
}

func (s *Server) webhookHandler(ctx *gin.Context) {
	body := ctx.Request.Body
	defer body.Close()

	var webhookPayload interface{}

	err := json.NewDecoder(body).Decode(&webhookPayload)
	if err != nil {
		slog.Error("Error decoding webhook payload", "error", err)
		ctx.Error(err)
		return
	}

	slog.Info("Received webhook payload", "webhookPayload", webhookPayload)
	ctx.JSON(200, gin.H{"status": "ok"})
}

// testTemplateHandler uses the payload as string and tries to parse it with the match template
func (s *Server) testTemplateHandler(ctx *gin.Context) {
	buf := new(bytes.Buffer)
	buf.ReadFrom(ctx.Request.Body)
	templateTextToTest := buf.String()

	testMatch := matchservice.MatchInfo{
		Id:   "test",
		MsID: "abc123",
		Team1: matchservice.Team{
			Id:    "def456",
			Name:  "Team1",
			Ready: true,
		},
		Team2: matchservice.Team{
			Id:    "ghi789",
			Name:  "Team2",
			Ready: true,
		},
		PlayerAmount:       4,
		Game:               "cs2",
		Map:                "de_dust2",
		ServerAddress:      "127.0.0.1:27015",
		ServerPassword:     "password",
		ServerPasswordMgmt: "rootpassword",
		ServerTvAddress:    "",
		ServerTvPassword:   "",
		TournamentName:     "nicematch",
		MatchTitle:         "Team1 vs Team2",
		Ready:              true,
	}

	parsedTmt2MatchTemplate, err := template.ParseTemplateForMatch(templateTextToTest, &testMatch)
	if err != nil {
		slog.Error("Error parsing template", "error", err)
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	slog.Debug("Parsed template", "parsedTmt2MatchTemplate", parsedTmt2MatchTemplate)

	// try to bind parsed template to tmt2 match

	var createMatchDto = tmt2_go.IMatchCreateDto{}
	err = json.Unmarshal([]byte(parsedTmt2MatchTemplate), &createMatchDto)
	if err != nil {
		slog.Error("Error unmarshalling parsed template", "error", err)
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(200, createMatchDto)
}

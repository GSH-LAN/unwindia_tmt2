package server

import (
	"context"
	"errors"
	"github.com/GSH-LAN/Unwindia_common/src/go/config"
	"github.com/GSH-LAN/Unwindia_common/src/go/workitemLock"
	"github.com/GSH-LAN/Unwindia_tmt2/pkg/tmt2-go"
	"github.com/GSH-LAN/Unwindia_tmt2/src/database"
	"github.com/GSH-LAN/Unwindia_tmt2/src/models"
	"github.com/GSH-LAN/Unwindia_tmt2/src/tmt2"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/gammazero/workerpool"
	"golang.org/x/sync/semaphore"
	"log/slog"
	"time"
)

type Worker struct {
	ctx            context.Context
	workerpool     *workerpool.WorkerPool
	dbClient       database.DatabaseClient
	matchPublisher message.Publisher
	semaphore      *semaphore.Weighted
	lock           workitemLock.WorkItemLock
	config         config.ConfigClient
	baseTopic      string
	tmt2Client     *tmt2.TMT2ClientImpl
	deleteWaitTime time.Duration
}

func NewWorker(ctx context.Context, pool *workerpool.WorkerPool, db database.DatabaseClient, matchPublisher message.Publisher, config config.ConfigClient, baseTopic string, tmt2Client *tmt2.TMT2ClientImpl, deleteWaitTime time.Duration) *Worker {
	w := Worker{
		ctx:            ctx,
		workerpool:     pool,
		dbClient:       db,
		matchPublisher: matchPublisher,
		semaphore:      semaphore.NewWeighted(int64(1)),
		lock:           workitemLock.NewMemoryWorkItemLock(),
		config:         config,
		baseTopic:      baseTopic,
		tmt2Client:     tmt2Client,
		deleteWaitTime: deleteWaitTime,
	}
	return &w
}

func (w *Worker) StartWorker(interval time.Duration) error {
	ticker := time.NewTicker(interval)
	for {
		select {
		case <-ticker.C:
			go w.process()
		case <-w.ctx.Done():
			return w.ctx.Err()
		}
	}
}

// process is the ticker routine that finds jobs which are ready to be processed
func (w *Worker) process() {
	slog.Debug("start processing")

	if !w.semaphore.TryAcquire(1) {
		slog.Warn("Skip processing, semaphore already acquired")
		return
	}
	defer w.semaphore.Release(1)

	matches, err := w.dbClient.List(w.ctx, nil)
	if err != nil {
		slog.Error("error retrieving new jobs from database", "Error", err)
		return
	}

	for _, match := range matches {
		if err = w.lock.Lock(w.ctx, match.MatchID, nil); err != nil {
			continue
		}
		defer w.lock.Unlock(w.ctx, match.MatchID)
		err = w.processMatch(w.ctx, match)
		if err != nil {
			slog.Error("error processing job", "Error", err)
		}
	}

	slog.Debug("finished job processing")
}

func (w *Worker) processMatch(ctx context.Context, match *database.Match) error {
	slog.Debug("start job processing", "Match", match.MatchID)

	switch match.JobState {
	case models.JOB_STATE_NEW:
		createMatchResponse, err := w.createTMT2Match(ctx, match)
		if err != nil {
			slog.Error("error creating tmt2 match", "Error", err)
			return err
		}
		slog.Info("created tmt2 match", "Match", match.MatchID, "Response", *createMatchResponse.JSON201)

		match.JobState = models.JOB_STATE_IN_PROGRESS
		match.TMT2MatchId = createMatchResponse.JSON201.Id
		_, err = w.dbClient.UpdateMatch(ctx, match)
		if err != nil {
			slog.Error("error updating match", "Error", err)
			return err
		}
	case models.JOB_STATE_IN_PROGRESS:
		slog.Debug("job in progress", "Match", match.MatchID)
		if match.FinishedAt != nil && match.FinishedAt.After(time.Time{}) && match.FinishedAt.Add(w.deleteWaitTime).Before(time.Now()) {
			match.JobState = models.JOB_STATE_FINISHED
			_, err := w.dbClient.UpdateMatch(ctx, match)
			if err != nil {
				slog.Error("error updating match", "Error", err)
				return err
			}
		}
	case models.JOB_STATE_FINISHED:
		slog.Debug("job already finished", "Match", match.MatchID)
		w.deleteTMT2Match(ctx, match)
	}

	return nil
}

func (w *Worker) createTMT2Match(ctx context.Context, match *database.Match) (*tmt2_go.CreateMatchResponse, error) {
	slog.Debug("createTMT2Match", "Match", match.MatchID)

	response, err := w.tmt2Client.CreateMatch(ctx, &match.MatchInfo)
	if err != nil {
		slog.Error("error creating tmt2 match", "Error", err)
		return nil, err
	}

	if response.StatusCode() > 299 {
		slog.Error("error creating tmt2 match", "Error", string(response.Body))

		return nil, errors.New("error creating tmt2 match: " + response.Status())
	}

	return response, nil
}

func (w *Worker) deleteTMT2Match(ctx context.Context, match *database.Match) error {
	slog.Debug("deleteTMT2Match", "Match", match.MatchID)

	err := w.tmt2Client.DeleteMatch(ctx, match.TMT2MatchId)
	if err != nil {
		slog.Error("error deleting tmt2 match", "Error", err)
		return err
	}

	return nil
}

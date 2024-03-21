package database

import (
	"context"
	"github.com/GSH-LAN/Unwindia_tmt2/src/environment"
	"github.com/kamva/mgm/v3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log/slog"
	"time"
)

const (
	DatabaseName   = "unwindia"
	DefaultTimeout = 10 * time.Second
)

// DatabaseClient is the client-interface for the main mongodb database
type DatabaseClient interface {
	// UpsertJob creates or updates an DotlanForumStatus entry
	CreateMatch(ctx context.Context, entry *Match) (string, error)
	UpdateMatch(ctx context.Context, entry *Match) (string, error)
	DeleteMatch(ctx context.Context, id string) error
	GetMatchByMatchID(ctx context.Context, id string) (*Match, error)
	List(ctx context.Context, filter interface{}) ([]*Match, error)
}

func NewClient(ctx context.Context, env *environment.Environment) (*DatabaseClientImpl, error) {
	client, err := mongo.NewClient(options.Client().ApplyURI(env.MongoDbURI))
	if err != nil {
		slog.Error("Error creating Mongo client", "Error", err)
		return nil, err
	}

	ctx, _ = context.WithTimeout(ctx, 10*time.Second)
	err = client.Connect(ctx)
	if err != nil {
		slog.Error("Error connecting to Mongo", "Error", err)
		return nil, err
	}

	err = mgm.SetDefaultConfig(nil, DatabaseName, options.Client().ApplyURI(env.MongoDbURI))
	if err != nil {
		slog.Error("Error creating mgm connection", "Error", err)
		return nil, err
	}

	dbClient := DatabaseClientImpl{
		ctx:                ctx,
		collection:         mgm.Coll(&Match{}),
		matchExpirationTTL: env.MatchExpirationTTL,
	}

	return &dbClient, err
}

type DatabaseClientImpl struct {
	ctx                context.Context
	collection         *mgm.Collection
	matchExpirationTTL time.Duration
}

func (d DatabaseClientImpl) CreateMatch(ctx context.Context, entry *Match) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	//// TTL index
	//index := mongo.IndexModel{
	//	Keys:    bson.D{{"created_at", 1}},
	//	Options: options.Index().SetExpireAfterSeconds(int32(d.matchExpirationTTL.Seconds())),
	//}
	//
	//_, err := d.collection.Indexes().CreateOne(ctx, index)
	//if err != nil {
	//	// TODO: recreate index if already existing
	//	slog.Error("Error creating index", "Error", err)
	//}

	err := d.collection.CreateWithCtx(ctx, entry)

	return entry.ID.String(), err
}

func (d DatabaseClientImpl) UpdateMatch(ctx context.Context, entry *Match) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	err := d.collection.UpdateWithCtx(ctx, entry)

	return entry.ID.String(), err
}

func (d DatabaseClientImpl) DeleteMatch(ctx context.Context, id string) error {
	//TODO implement me
	panic("implement me")
}

func (d DatabaseClientImpl) GetMatchByMatchID(ctx context.Context, id string) (*Match, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	filter := bson.D{{"match_id", id}}
	result := d.collection.FindOne(ctx, filter)
	if result.Err() != nil {
		return nil, result.Err()
	}

	var entry Match
	err := result.Decode(&entry)
	if err != nil {
		return nil, err
	}

	return &entry, nil
}

func (d DatabaseClientImpl) List(ctx context.Context, filter interface{}) ([]*Match, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	if filter == nil {
		filter = bson.D{}
	}

	cur, err := d.collection.Find(ctx, filter)

	if err != nil {
		return nil, err
	}

	var jobs []*Match

	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var result Match
		if err = cur.Decode(&result); err != nil {
			slog.Error("Error decoding match", "Error", err)
		} else {
			jobs = append(jobs, &result)
		}
	}

	return jobs, nil
}

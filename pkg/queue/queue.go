package queue

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/trusch/deadman-switch/pkg/concurrency"
	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/mvcc/mvccpb"
)

var (
	ErrQueueEmpty error = errors.New("queue is empty")
)

type Queue interface {
	Enqueue(ctx context.Context, data interface{}) error
	Dequeue(ctx context.Context, data interface{}) error
}

func NewEtcdQueue(ctx context.Context, cli *clientv3.Client, prefix string) (Queue, error) {
	concurrencyClient, err := concurrency.NewEtcdClient(ctx, cli)
	if err != nil {
		return nil, err
	}
	q := &etcdQueue{
		prefix:      prefix,
		cli:         cli,
		concurrency: concurrencyClient,
	}
	return q, nil
}

type etcdQueue struct {
	prefix      string
	cli         *clientv3.Client
	concurrency concurrency.Client
}

func (q *etcdQueue) Enqueue(ctx context.Context, obj interface{}) error {
	now := time.Now().Format(time.RFC3339Nano)
	key := filepath.Join(q.prefix, "items", now)
	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	log.Debug().Interface("obj", obj).Msg("enqueue stuff")
	_, err = q.cli.Put(ctx, key, string(data))
	if err != nil {
		return err
	}
	return nil
}

func (q *etcdQueue) Dequeue(ctx context.Context, target interface{}) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	err := q.concurrency.Lock(ctx, filepath.Join(q.prefix, "queue"))
	if err != nil {
		return err
	}
	key := filepath.Join(q.prefix, "items")
	var kv *mvccpb.KeyValue
	resp, err := q.cli.KV.Get(ctx, key, append(clientv3.WithFirstKey(), clientv3.WithPrefix())...)
	if err != nil {
		return err
	}
	if len(resp.Kvs) > 0 {
		kv = resp.Kvs[0]
	} else {
		ctx, cancel := context.WithCancel(ctx)
		ch := q.cli.Watch(ctx, key, clientv3.WithPrefix())
		watchResp, ok := <-ch
		if !ok {
			cancel()
			return ErrQueueEmpty
		}
		cancel()
		kv = watchResp.Events[0].Kv
	}
	err = json.Unmarshal(kv.Value, target)
	if err != nil {
		return err
	}
	_, err = q.cli.KV.Delete(ctx, string(kv.Key))
	if err != nil {
		return err
	}
	return nil
}

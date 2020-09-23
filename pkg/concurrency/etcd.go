package concurrency

import (
	"context"
	"time"

	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/clientv3/concurrency"
)

func NewEtcdClient(ctx context.Context, cli *clientv3.Client) (Client, error) {
	lease, err := cli.Grant(ctx, 5)
	if err != nil {
		return nil, err
	}
	ch, err := cli.KeepAlive(ctx, lease.ID)
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ch:
				continue
			}
		}
	}()
	session, err := concurrency.NewSession(cli, concurrency.WithLease(lease.ID))
	if err != nil {
		return nil, err
	}
	election := concurrency.NewElection(session, "/deadman-switch-leader")
	return &etcdClient{
		cli:      cli,
		lease:    lease.ID,
		session:  session,
		election: election,
	}, nil
}

type etcdClient struct {
	cli      *clientv3.Client
	lease    clientv3.LeaseID
	session  *concurrency.Session
	election *concurrency.Election
}

func (c *etcdClient) IsLeader(ctx context.Context, id string) (bool, error) {
	if c.election.Key() == id {
		return true, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	err := c.election.Campaign(ctx, id)
	if err != nil {
		if err == context.Canceled {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *etcdClient) Lock(ctx context.Context, key string) error {
	mutex := concurrency.NewMutex(c.session, key)
	err := mutex.Lock(ctx)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		mutex.Unlock(c.cli.Ctx())
	}()
	return nil
}

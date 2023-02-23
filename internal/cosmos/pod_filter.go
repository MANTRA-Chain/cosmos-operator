package cosmos

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
)

// TendermintStatuser calls the Tendermint RPC status endpoint.
type TendermintStatuser interface {
	Status(ctx context.Context, rpcHost string) (TendermintStatus, error)
}

// PodFilter queries tendermint for block heights.
type PodFilter struct {
	tendermint TendermintStatuser
}

func NewPodFilter(status TendermintStatuser) *PodFilter {
	return &PodFilter{
		tendermint: status,
	}
}

// SyncedPods returns all pods that are in sync (i.e. no longer catching up).
// Caller is responsible for timeouts via the context.
func (filter PodFilter) SyncedPods(ctx context.Context, log logr.Logger, candidates []*corev1.Pod) []*corev1.Pod {
	var (
		eg     errgroup.Group
		inSync = make([]*corev1.Pod, len(candidates))
	)

	for i := range candidates {
		i := i
		eg.Go(func() error {
			pod := candidates[i]
			logger := log.WithValues("pod", pod.Name)
			ip := pod.Status.PodIP
			if ip == "" {
				logger.Info("Pod has no IP")
				return nil
			}
			host := fmt.Sprintf("http://%s:26657", ip)
			resp, err := filter.tendermint.Status(ctx, host)
			if err != nil {
				logger.Info("Failed to fetch tendermint rpc status", "error", err)
				return nil
			}
			if resp.Result.SyncInfo.CatchingUp {
				logger.Info("Pod is still catching up")
				return nil
			}
			inSync[i] = pod
			return nil
		})
	}

	_ = eg.Wait()

	return lo.Compact(inSync)
}
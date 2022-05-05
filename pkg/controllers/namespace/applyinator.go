package namespace

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
)

type ApplyFunc func(key string) error

type Applyinator interface {
	Apply(key string)
	Run(ctx context.Context, workers int)
}

// name "project-registration-namespace-workqueue"
func NewApplyinator(name string, applyFunc ApplyFunc) Applyinator {
	return &applyinator{
		workqueue: workqueue.NewNamedRateLimitingQueue(
			workqueue.NewMaxOfRateLimiter(
				workqueue.NewItemFastSlowRateLimiter(time.Millisecond, 2*time.Minute, 30),
				workqueue.NewItemExponentialFailureRateLimiter(5*time.Millisecond, 30*time.Second),
			), name,
		),
		apply: applyFunc,
	}
}

type applyinator struct {
	workqueue workqueue.RateLimitingInterface
	apply     ApplyFunc
}

func (a *applyinator) Apply(key string) {
	a.workqueue.Add(key)
}

func (a *applyinator) Run(ctx context.Context, workers int) {
	go func() {
		<-ctx.Done()
		a.workqueue.ShutDown()
	}()
	for i := 0; i < workers; i++ {
		go wait.Until(a.runWorker, time.Second, ctx.Done())
	}
}

func (a *applyinator) runWorker() {
	for a.processNextWorkItem() {
	}
}

func (a *applyinator) processNextWorkItem() bool {
	obj, shutdown := a.workqueue.Get()

	if shutdown {
		return false
	}

	if err := a.processSingleItem(obj); err != nil {
		if !strings.Contains(err.Error(), "please apply your changes to the latest version and try again") {
			logrus.Errorf("%v", err)
		}
		return true
	}

	return true
}

func (a *applyinator) processSingleItem(obj interface{}) error {
	var (
		key string
		ok  bool
	)

	defer a.workqueue.Done(obj)

	if key, ok = obj.(string); !ok {
		a.workqueue.Forget(obj)
		logrus.Errorf("expected string in workqueue but got %#v", obj)
		return nil
	}
	if err := a.apply(key); err != nil {
		a.workqueue.AddRateLimited(key)
		return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
	}

	a.workqueue.Forget(obj)
	return nil
}

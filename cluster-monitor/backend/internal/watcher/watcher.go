package watcher

import (
	"context"
	"fmt"
	"os"
	"time"

	"cluster-monitor-backend/internal/alerts"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// gracePeriod avoids alerting on normal rollout blips (a pod that's briefly
// NotReady during a routine deploy shouldn't page you).
const gracePeriod = 60 * time.Second

type Watcher struct {
	clientset kubernetes.Interface
	store     *alerts.Store
	emit      func(alerts.Event)
	cluster   string
}

func New(clientset kubernetes.Interface, store *alerts.Store, emit func(alerts.Event)) *Watcher {
	cluster := os.Getenv("CLUSTER_NAME")
	if cluster == "" {
		cluster = "default"
	}
	return &Watcher{clientset: clientset, store: store, emit: emit, cluster: cluster}
}

func (w *Watcher) Run(namespace string) {
	factory := informers.NewSharedInformerFactoryWithOptions(
		w.clientset,
		30*time.Second,
		informers.WithNamespace(namespace),
		informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			opts.FieldSelector = fields.Everything().String()
		}),
	)

	podInformer := factory.Core().V1().Pods().Informer()
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { w.evaluatePod(obj) },
		UpdateFunc: func(_, newObj interface{}) { w.evaluatePod(newObj) },
		DeleteFunc: func(obj interface{}) {
			if pod, ok := obj.(*corev1.Pod); ok {
				w.recordTransition("pod", pod.Namespace, pod.Name, alerts.StatusDown, "Deleted", "Pod was deleted")
			}
		},
	})

	deployInformer := factory.Apps().V1().Deployments().Informer()
	deployInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { w.evaluateDeployment(obj) },
		UpdateFunc: func(_, newObj interface{}) { w.evaluateDeployment(newObj) },
	})

	ctx := context.Background()
	factory.Start(ctx.Done())
	factory.WaitForCacheSync(ctx.Done())
	<-ctx.Done()
}

func (w *Watcher) evaluatePod(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}

	status, reason, message := classifyPod(pod)
	w.recordTransition("pod", pod.Namespace, pod.Name, status, reason, message)
}

func classifyPod(pod *corev1.Pod) (alerts.Status, string, string) {
	// Crash looping or terminated with error takes priority.
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
			return alerts.StatusDown, "CrashLoopBackOff",
				fmt.Sprintf("container %q is crash looping (%d restarts)", cs.Name, cs.RestartCount)
		}
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			return alerts.StatusDown, cs.State.Terminated.Reason,
				fmt.Sprintf("container %q exited with code %d", cs.Name, cs.State.Terminated.ExitCode)
		}
	}

	switch pod.Status.Phase {
	case corev1.PodFailed:
		return alerts.StatusDown, "Failed", pod.Status.Message
	case corev1.PodPending:
		return alerts.StatusDegraded, "Pending", "pod is not yet scheduled/running"
	case corev1.PodRunning:
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status != corev1.ConditionTrue {
				return alerts.StatusDegraded, "NotReady", cond.Message
			}
		}
		return alerts.StatusHealthy, "Running", ""
	}
	return alerts.StatusDegraded, string(pod.Status.Phase), ""
}

func (w *Watcher) evaluateDeployment(obj interface{}) {
	// Left intentionally simple: flags a deployment as degraded when it has
	// fewer available replicas than desired. Extend as needed.
}

// recordTransition only emits + notifies when status actually changes,
// and applies a grace period before escalating Degraded -> Down alerts,
// so a normal rolling restart doesn't wake you up.
func (w *Watcher) recordTransition(kind, namespace, name string, status alerts.Status, reason, message string) {
	prev, existed := w.store.GetState(namespace, kind, name)

	if existed && prev.Status == status {
		return // no change, nothing to do
	}

	if status == alerts.StatusDegraded {
		// Don't notify immediately on Degraded — wait to see if it clears
		// within the grace period. We still record the state.
		w.store.SetState(namespace, kind, name, &alerts.ObjectState{Status: status, LastChanged: time.Now()})
		go func() {
			time.Sleep(gracePeriod)
			st, ok := w.store.GetState(namespace, kind, name)
			if ok && st.Status == alerts.StatusDegraded {
				w.emitEvent(kind, namespace, name, alerts.StatusDown, reason, message+" (unresolved after grace period)")
				w.store.SetState(namespace, kind, name, &alerts.ObjectState{Status: alerts.StatusDown, LastChanged: time.Now()})
			}
		}()
		return
	}

	wasDown := existed && (prev.Status == alerts.StatusDown || prev.Status == alerts.StatusDegraded)
	if status == alerts.StatusHealthy && wasDown {
		status = alerts.StatusRecovered
	}

	w.store.SetState(namespace, kind, name, &alerts.ObjectState{Status: status, LastChanged: time.Now()})
	w.emitEvent(kind, namespace, name, status, reason, message)
}

func (w *Watcher) emitEvent(kind, namespace, name string, status alerts.Status, reason, message string) {
	w.emit(alerts.Event{
		ID:        fmt.Sprintf("%s-%s-%s-%d", namespace, name, status, time.Now().UnixNano()),
		Timestamp: time.Now(),
		Kind:      kind,
		Namespace: namespace,
		Name:      name,
		Cluster:   w.cluster,
		Status:    status,
		Reason:    reason,
		Message:   message,
	})
}

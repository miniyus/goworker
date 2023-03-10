package goworker

import (
	"encoding/json"
	"fmt"
	"github.com/redis/go-redis/v9"
	"log"
	"time"
)

type Logger interface {
	Info(args ...interface{})

	//Infof implements message with Sprint, Sprintf, or neither.
	Infof(template string, args ...interface{})
	Infoln(args ...interface{})
	Error(args ...interface{})
	Errorf(template string, args ...interface{})
	Errorln(args ...interface{})
	Debug(args ...interface{})
	Debugf(template string, args ...interface{})
	Debugln(args ...interface{})
	Warn(args ...interface{})
	Warnf(template string, args ...interface{})
	Warnln(args ...interface{})
}

// Dispatcher dispatcher
// manage workers
type Dispatcher interface {
	Dispatch(jobId string, closure func(j *Job) error) error
	Run(names ...string)
	Stop(names ...string)
	SelectWorker(name string) Dispatcher
	GetWorkers() []Worker
	GetRedis() func() *redis.Client
	AddWorker(option Option)
	RemoveWorker(nam string)
	Status() *StatusInfo
	BeforeJob(fn func(j *Job) error, workerNames ...string)
	AfterJob(fn func(j *Job, err error) error, workerNames ...string)
	OnDispatch(fn func(j *Job) error, workerNames ...string)
}

// JobDispatcher implements Dispatcher
type JobDispatcher struct {
	workers []Worker
	worker  Worker
	redis   func() *redis.Client
	logger  Logger
}

// Option JobWorker's option
type Option struct {
	Name        string
	MaxJobCount int
	BeforeJob   func(j *Job) error
	AfterJob    func(j *Job, err error) error
	Delay       time.Duration
	Logger      Logger
}

// DispatcherOption dispatcher option
type DispatcherOption struct {
	WorkerOptions []Option
	Redis         func() *redis.Client
}

// defaultWorkerOption default option setting
var defaultWorkerOption = []Option{
	{
		Name:        DefaultWorker,
		MaxJobCount: 10,
	},
}

// NewDispatcher make dispatcher
func NewDispatcher(opt DispatcherOption) Dispatcher {
	workers := make([]Worker, 0)

	if len(opt.WorkerOptions) == 0 {
		opt.WorkerOptions = defaultWorkerOption
	}

	for _, o := range opt.WorkerOptions {
		workers = append(workers, NewWorker(Config{
			o.Name,
			opt.Redis,
			o.MaxJobCount,
			o.BeforeJob,
			o.AfterJob,
			o.Delay,
			o.Logger,
		}))
	}

	return &JobDispatcher{
		workers: workers,
		worker:  nil,
		redis:   opt.Redis,
	}
}

// AddWorker add worker in runtime
func (d *JobDispatcher) AddWorker(option Option) {
	d.workers = append(d.workers, NewWorker(Config{
		option.Name,
		d.redis,
		option.MaxJobCount,
		option.BeforeJob,
		option.AfterJob,
		option.Delay,
		option.Logger,
	}))
}

// RemoveWorker remove worker in runtime
func (d *JobDispatcher) RemoveWorker(name string) {
	var rmIndex *int = nil
	for i, worker := range d.workers {
		if worker.GetName() == name {
			rmIndex = &i
		}
	}

	if rmIndex != nil {
		d.workers = append(d.workers[:*rmIndex], d.workers[*rmIndex+1:]...)
	}
}

// GetRedis redis client make function
func (d *JobDispatcher) GetRedis() func() *redis.Client {
	return d.redis
}

// GetWorkers get this dispatcher's workers
func (d *JobDispatcher) GetWorkers() []Worker {
	return d.workers
}

// SelectWorker select worker by worker name
func (d *JobDispatcher) SelectWorker(name string) Dispatcher {
	if name == "" {
		for _, w := range d.workers {
			if w.GetName() == "default" {
				d.worker = w
			}
		}

	}

	for _, w := range d.workers {
		if w.GetName() == name {
			d.worker = w
		}
	}

	return d
}

// BeforeJob ?????? Job ?????? ??? ????????? ????????? ??????
func (d *JobDispatcher) BeforeJob(fn func(j *Job) error, workerNames ...string) {
	if len(workerNames) == 0 {
		for _, w := range d.workers {
			w.BeforeJob(fn)
		}
	} else {
		for _, w := range d.workers {
			for _, name := range workerNames {
				if w.GetName() == name {
					w.BeforeJob(fn)
				}
			}
		}
	}
}

// AfterJob ?????? Job ?????? ??? ????????? ????????? ??????, error??? ????????? ?????? ?????? ????????? error??? ?????? ?????? ?????????.
func (d *JobDispatcher) AfterJob(fn func(j *Job, err error) error, workerNames ...string) {
	if len(workerNames) == 0 {
		for _, w := range d.workers {
			w.AfterJob(fn)
		}
	} else {
		for _, w := range d.workers {
			for _, name := range workerNames {
				if w.GetName() == name {
					w.AfterJob(fn)
				}
			}
		}
	}
}

func (d *JobDispatcher) OnDispatch(fn func(j *Job) error, workerNames ...string) {
	var workers []Worker

	if len(workerNames) == 0 {
		workers = d.workers
	} else {
		for _, w := range d.workers {
			for _, wn := range workerNames {
				if wn == w.GetName() {
					workers = append(workers, w)
				}
			}
		}
	}

	for _, w := range workers {
		w.OnAddJob(fn)
	}
}

// Dispatch job??? ???????????? worker??? ???????????? ????????? ????????? ??????.
func (d *JobDispatcher) Dispatch(jobId string, closure func(j *Job) error) error {
	if d.worker == nil {
		for _, w := range d.workers {
			if w.GetName() == DefaultWorker {
				d.worker = w
			}
		}
	}

	err := d.worker.AddJob(newJob(d.worker.GetName(), jobId, closure))
	if err != nil {
		return err
	}

	return nil
}

// Run dispatcher??? worker?????? ?????? ?????? ????????? ????????? ??? ??? ?????? ??????,
// ?????? ???????????? ???????????? ?????? ?????? workerNames ??????????????? ??????
func (d *JobDispatcher) Run(workerNames ...string) {
	workers := make([]Worker, 0)

	if len(workerNames) == 0 {
		workers = d.workers
	} else {
		for _, w := range d.workers {
			for _, wn := range workerNames {
				if wn == w.GetName() {
					workers = append(workers, w)
				}
			}
		}
	}

	for _, w := range workers {
		w.Run()
	}
}

// Stop ?????? worker??? ????????? ????????????.
// ????????? ???????????? ?????? ?????? workerNames ??????????????? ??????
func (d *JobDispatcher) Stop(workerNames ...string) {
	workers := make([]Worker, 0)

	if len(workerNames) == 0 {
		workers = d.workers
	} else {
		for _, w := range d.workers {
			for _, wn := range workerNames {
				if wn == w.GetName() {
					workers = append(workers, w)
				}
			}
		}
	}

	for _, w := range workers {
		w.Stop()
	}
}

type StatusWorkerInfo struct {
	Name        string `json:"name"`
	IsRunning   bool   `json:"is_running"`
	JobCount    int    `json:"job_count"`
	MaxJobCount int    `json:"max_job_count"`
}

type StatusInfo struct {
	Workers     []StatusWorkerInfo `json:"workers"`
	WorkerCount int                `json:"worker_count"`
}

// Print StatusInfo to Console log
func (si *StatusInfo) Print() {
	for _, w := range si.Workers {
		prefix := fmt.Sprintf("[worker: %s]", w.Name)
		marshal, err := json.Marshal(w)
		if err != nil {
			log.Printf("%s %v", prefix, err)
		} else {
			log.Printf("%s %s", prefix, string(marshal))
		}

	}
}

// Status ?????? worker?????? ????????? ????????????.
func (d *JobDispatcher) Status() *StatusInfo {

	workers := make([]StatusWorkerInfo, 0)
	for _, w := range d.workers {
		workerInfo := StatusWorkerInfo{
			Name:        w.GetName(),
			IsRunning:   w.IsRunning(),
			JobCount:    w.JobCount(),
			MaxJobCount: w.MaxJobCount(),
		}

		workers = append(workers, workerInfo)
	}

	return &StatusInfo{
		Workers:     workers,
		WorkerCount: len(workers),
	}
}

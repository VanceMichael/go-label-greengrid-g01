package worker

import "sync"

func waitForWorkers(wg *sync.WaitGroup, done chan<- struct{}) {
	wg.Wait()
	close(done)
}

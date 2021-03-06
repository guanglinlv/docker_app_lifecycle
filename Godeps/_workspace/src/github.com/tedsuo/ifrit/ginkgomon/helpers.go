package ginkgomon

import (
	"fmt"
	"os"

	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/gomega"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/tedsuo/ifrit"
)

func Invoke(runner ifrit.Runner) ifrit.Process {
	process := ifrit.Background(runner)

	select {
	case <-process.Ready():
	case err := <-process.Wait():
		ginkgo.Fail(fmt.Sprintf("process failed to start: %s", err))
	}

	return process
}

func Interrupt(process ifrit.Process, intervals ...interface{}) {
	process.Signal(os.Interrupt)
	Eventually(process.Wait(), intervals...).Should(Receive(), "interrupted ginkgomon process failed to exit in time")
}

func Kill(process ifrit.Process, intervals ...interface{}) {
	if process != nil {
		process.Signal(os.Kill)
		Eventually(process.Wait(), intervals...).Should(Receive(), "killed ginkgomon process failed to exit in time")
	}
}

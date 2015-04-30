package main_test

import (
	"errors"
	"io/ioutil"
	"os"
	"strings"
	"time"

	. "github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/gomega"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/tedsuo/ifrit"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/tedsuo/ifrit/ginkgomon"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/tedsuo/ifrit/grouper"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/builder"
)

var _ = Describe("Builder runner", func() {
	var (
		lifecycle        ifrit.Process
		fakeDeamonRunner func(signals <-chan os.Signal, ready chan<- struct{}) error
	)

	BeforeEach(func() {
		builder := main.Builder{
			RepoName:            "ubuntu",
			Tag:                 "latest",
			OutputFilename:      "/tmp/result/result.json",
			DockerDaemonTimeout: 300 * time.Millisecond,
			CacheDockerImage:    true,
		}

		lifecycle = ifrit.Background(grouper.NewParallel(os.Interrupt, grouper.Members{
			{"builder", ifrit.RunFunc(builder.Run)},
			{"fake_docker_daemon", ifrit.RunFunc(fakeDeamonRunner)},
		}))
	})

	AfterEach(func() {
		ginkgomon.Interrupt(lifecycle)
	})

	Context("when the daemon won't start", func() {
		fakeDeamonRunner = func(signals <-chan os.Signal, ready chan<- struct{}) error {
			close(ready)
			select {
			case signal := <-signals:
				return errors.New(signal.String())
			case <-time.After(1 * time.Second):
				// Daemon "crashes" after a while
			}
			return nil
		}

		It("times out", func() {
			err := <-lifecycle.Wait()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Timed out waiting for docker daemon to start"))
		})

		Context("and the process is interrupted", func() {
			BeforeEach(func() {
				lifecycle.Signal(os.Interrupt)
			})

			It("exists with error", func() {
				err := <-lifecycle.Wait()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake_docker_daemon exited with error: interrupt"))
				Expect(err.Error()).To(ContainSubstring("builder exited with error: interrupt"))
			})
		})
	})

	Describe("cached tags generation", func() {
		var (
			builder                 main.Builder
			dockerRegistryAddresses []string
		)

		generateTag := func() (string, string) {
			image, err := builder.GenerateImageName()
			Expect(err).NotTo(HaveOccurred())

			parts := strings.Split(image, "/")
			Expect(parts).To(HaveLen(2))
			Expect(dockerRegistryAddresses).To(ContainElement(parts[0]))

			return parts[0], parts[1]
		}

		imageGeneration := func() {
			generatedImageNames := make(map[string]int)

			uniqueImageNames := func() bool {
				_, imageName := generateTag()
				generatedImageNames[imageName]++

				for key := range generatedImageNames {
					if generatedImageNames[key] != 1 {
						return false
					}
				}

				return true
			}

			It("generates different image names", func() {
				Consistently(uniqueImageNames).Should(BeTrue())
			})
		}

		BeforeEach(func() {
			builder = main.Builder{
				DockerRegistryAddresses: dockerRegistryAddresses,
			}
		})

		Context("when there are several Docker Registry addresses", func() {
			dockerRegistryAddresses = []string{"one", "two", "three", "four"}

			Describe("addresses", func() {
				generatedAddresses := make(map[string]bool, len(dockerRegistryAddresses))

				allAddressesSelected := func() bool {
					address, _ := generateTag()
					generatedAddresses[address] = true

					for _, address := range dockerRegistryAddresses {
						if !generatedAddresses[address] {
							return false
						}
					}
					return true
				}

				It("selects all addresses", func() {
					Eventually(allAddressesSelected).Should(BeTrue())
				})
			})

			Describe("image names", imageGeneration)
		})

		Context("when there is a single Docker Registry address", func() {
			dockerRegistryAddresses = []string{"one"}

			Describe("image names", imageGeneration)
		})

	})

	Describe("docker config", func() {
		var builder main.Builder
		confFile := os.Getenv("HOME") + "/.dockercfg"

		AfterEach(func() {
			os.Remove(confFile)
		})

		Context("with correct data", func() {

			BeforeEach(func() {
				builder = main.Builder{
					DockerLoginServer:       "server",
					DockerRegistryEmail:     "email",
					DockerRegistryAuthToken: "token",
				}
				err := builder.WriteDockerConfig()
				Expect(err).ToNot(HaveOccurred())
			})

			It("writes the config", func() {
				bytes, err := ioutil.ReadFile(confFile)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(bytes)).To(Equal(`{"server": {"auth": "token", "email": "email"} }`))
			})
		})

		Context("with missing data", func() {
			BeforeEach(func() {
				builder = main.Builder{
					DockerLoginServer:       "server",
					DockerRegistryAuthToken: "token",
				}
			})

			It("does not write a config", func() {
				_, err := os.Stat(confFile)
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})

	})
})

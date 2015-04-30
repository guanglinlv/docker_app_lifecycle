package main

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/tedsuo/ifrit"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/tedsuo/ifrit/grouper"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/tedsuo/ifrit/sigmon"

	"github.com/cloudfoundry-incubator/docker_app_lifecycle/helpers"
)

type registries []string

func main() {
	var insecureDockerRegistries registries
	var dockerRegistryAddresses registries

	flagSet := flag.NewFlagSet("builder", flag.ExitOnError)

	dockerImageURL := flagSet.String(
		"dockerImageURL",
		"",
		"docker image uri in docker://[registry/][scope/]repository[#tag] format",
	)

	dockerRef := flagSet.String(
		"dockerRef",
		"",
		"docker image reference in standard docker string format",
	)

	outputFilename := flagSet.String(
		"outputMetadataJSONFilename",
		"/tmp/result/result.json",
		"filename in which to write the app metadata",
	)

	flagSet.Var(
		&insecureDockerRegistries,
		"insecureDockerRegistries",
		"Insecure Docker Registry addresses (ip:port)",
	)

	flagSet.Var(
		&dockerRegistryAddresses,
		"dockerRegistryAddresses",
		"Docker Registry addresses (ip:port)",
	)

	dockerDaemonExecutablePath := flagSet.String(
		"dockerDaemonExecutablePath",
		"/tmp/docker_app_lifecycle/docker",
		"path to the 'docker' executable",
	)

	cacheDockerImage := flagSet.Bool(
		"cacheDockerImage",
		false,
		"Caches Docker images to private docker registry",
	)

	dockerLoginServer := flagSet.String(
		"dockerLoginServer",
		"",
		"Docker Login server address",
	)

	dockerRegistryAuthToken := flagSet.String(
		"dockerRegistryAuthToken",
		"",
		"Authentication token for pulling from docker registry",
	)

	dockerRegistryEmail := flagSet.String(
		"dockerRegistryEmail",
		"",
		"Email for pulling from docker registry",
	)

	if err := flagSet.Parse(os.Args[1:len(os.Args)]); err != nil {
		println(err.Error())
		os.Exit(1)
	}

	var repoName string
	var tag string
	if len(*dockerImageURL) > 0 {
		parts, err := url.Parse(*dockerImageURL)
		if err != nil {
			println("invalid dockerImageURL: " + *dockerImageURL)
			flagSet.PrintDefaults()
			os.Exit(1)
		}
		repoName, tag = helpers.ParseDockerURL(parts)
	} else if len(*dockerRef) > 0 {
		repoName, tag = helpers.ParseDockerRef(*dockerRef)
	} else {
		println("missing flag: dockerImageURL or dockerRef required")
		flagSet.PrintDefaults()
		os.Exit(1)
	}

	builder := Builder{
		RepoName: repoName,
		Tag:      tag,
		DockerRegistryAddresses:    dockerRegistryAddresses,
		InsecureDockerRegistries:   insecureDockerRegistries,
		OutputFilename:             *outputFilename,
		DockerDaemonExecutablePath: *dockerDaemonExecutablePath,
		DockerDaemonTimeout:        10 * time.Second,
		CacheDockerImage:           *cacheDockerImage,
		DockerLoginServer:          *dockerLoginServer,
		DockerRegistryAuthToken:    *dockerRegistryAuthToken,
		DockerRegistryEmail:        *dockerRegistryEmail,
	}

	members := grouper.Members{
		{"builder", ifrit.RunFunc(builder.Run)},
	}

	if *cacheDockerImage {
		if len(dockerRegistryAddresses) == 0 {
			println("missing flag: dockerRegistryAddresses required")
			os.Exit(1)
		}

		validateCredentials(*dockerLoginServer, *dockerRegistryAuthToken, *dockerRegistryEmail)

		if _, err := os.Stat(*dockerDaemonExecutablePath); err != nil {
			println("docker daemon not found in", *dockerDaemonExecutablePath)
			os.Exit(1)
		}

		dockerDaemon := DockerDaemon{
			DockerDaemonPath:         *dockerDaemonExecutablePath,
			InsecureDockerRegistries: insecureDockerRegistries,
		}
		members = append(members, grouper.Member{"docker_daemon", ifrit.RunFunc(dockerDaemon.Run)})
	}

	group := grouper.NewParallel(os.Interrupt, members)
	process := ifrit.Invoke(sigmon.New(group))

	fmt.Println("Staging process started ...")

	err := <-process.Wait()
	if err != nil {
		println("Staging process failed:", err.Error())
		os.Exit(2)
	}

	fmt.Println("Staging process finished")
}

func validateCredentials(server, token, email string) {
	missing := len(server) == 0 && len(token) == 0 && len(email) == 0
	present := len(server) > 0 && len(token) > 0 && len(email) > 0

	if missing {
		return
	}

	if !present {
		println("missing flags: dockerLoginServer, dockerRegistryAuthToken and dockerRegistryEmail required simultaneously")
		os.Exit(1)
	}

	_, err := url.Parse(server)
	if err != nil {
		println(fmt.Sprintf("invalid dockerLoginServer [%s]", server))
		os.Exit(1)
	}
	if !(strings.Contains(email, "@") && strings.Contains(email, ".")) {
		println(fmt.Sprintf("invalid dockerRegistryEmail [%s]", email))
		os.Exit(1)
	}
}

func (r *registries) String() string {
	return fmt.Sprint(*r)
}

func (r *registries) Set(value string) error {
	if len(*r) > 0 {
		return errors.New("Docker Registries flag already set")
	}
	for _, reg := range strings.Split(value, ",") {
		registry := strings.TrimSpace(reg)
		if strings.Contains(registry, "://") {
			return errors.New("no scheme allowed for Docker Registry [" + registry + "]")
		}
		if !strings.Contains(registry, ":") {
			return errors.New("ip:port expected for Docker Registry [" + registry + "]")
		}
		*r = append(*r, registry)
	}
	return nil
}

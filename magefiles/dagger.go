//go:build mage
// +build mage

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"dagger.io/dagger"
	"github.com/containers/image/v5/docker/reference"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

const (
	// https://hub.docker.com/_/golang/tags?page=1&name=alpine
	alpineVersion = "3.17"
	goVersion     = "1.20"

	// https://github.com/golangci/golangci-lint/releases
	golangciLintVersion = "1.51.2"

	// https://hub.docker.com/r/flyio/flyctl/tags
	flyctlVersion = "0.0.482"

	binaryName = "registry-redirect"
	_imageName = "registry.fly.io/dagger-registry-2023-01-23"

	InstancesToDeploy = "3"
	// We want to avoid running multiple instances in the same region
	// If there are issues with one region, the whole service will be disrupted
	MaxInstancesPerRegion = "1"

	// https://fly.io/docs/reference/regions/#fly-io-regions
	Amsterdam = "ams"
	Chicago   = "ord"
	Paris     = "cdg"
	Singapore = "sin"
	Sunnyvale = "sjc"

	// https://fly.io/docs/reference/configuration/#picking-a-deployment-strategy
	DeployStrategy = "canary"
)

// golangci-lint
func Lint(ctx context.Context) {
	defer handleErr()

	d := daggerClient(ctx)
	defer d.Close()

	lint(ctx, d)
}

func lint(ctx context.Context, d *dagger.Client) {
	exitCode, err := d.Container().
		From(fmt.Sprintf("golangci/golangci-lint:v%s-alpine", golangciLintVersion)).
		WithMountedCache("/go/pkg/mod", d.CacheVolume("gomod")).
		WithMountedDirectory("/src", sourceCode(d)).WithWorkdir("/src").
		WithExec([]string{"golangci-lint", "run", "--color", "always", "--timeout", "2m"}).
		ExitCode(ctx)

	if err != nil {
		panic(unavailableErr(err))
	}

	if exitCode != 0 {
		panic(Exit{Code: exitCode, Error: err})
	}
}

// go test
func Test(ctx context.Context) {
	defer handleErr()

	d := daggerClient(ctx)
	defer d.Close()

	test(ctx, d)
}

func test(ctx context.Context, d *dagger.Client) {
	exitCode, err := d.Container().
		From(fmt.Sprintf("golang:%s-alpine%s", goVersion, alpineVersion)).
		WithMountedDirectory("/src", sourceCode(d)).WithWorkdir("/src").
		WithMountedCache("/go/pkg/mod", d.CacheVolume("gomod")).
		WithEnvVariable("CGO_ENABLED", "0").
		WithExec([]string{"go", "test", "./..."}).
		ExitCode(ctx)

	if err != nil {
		panic(unavailableErr(err))
	}

	if exitCode != 0 {
		panic(Exit{Code: exitCode, Error: err})
	}
}

// binary artefact used in container image
func Build(ctx context.Context) {
	defer handleErr()

	d := daggerClient(ctx)
	defer d.Close()

	_ = build(ctx, d)
}

func build(ctx context.Context, d *dagger.Client) *dagger.File {
	binaryPath := fmt.Sprintf("build/%s", binaryName)

	buildBinary := d.Container().
		From(fmt.Sprintf("golang:%s-alpine%s", goVersion, alpineVersion)).
		WithMountedDirectory("/src", sourceCode(d)).WithWorkdir("/src").
		WithMountedCache("/go/pkg/mod", d.CacheVolume("gomod")).
		WithExec([]string{"go", "build", "-o", binaryPath})

	_, err := buildBinary.ExitCode(ctx)
	if err != nil {
		panic(createErr(err))
	}

	return buildBinary.File(binaryPath)
}

func sourceCode(d *dagger.Client) *dagger.Directory {
	return d.Host().Directory(".", dagger.HostDirectoryOpts{
		Include: []string{
			"LICENSE",
			"README.md",
			"go.mod",
			"go.sum",
			"**/*.go",
		},
	})
}

func deployConfig(d *dagger.Client) *dagger.File {
	return d.Host().Directory(".", dagger.HostDirectoryOpts{
		Include: []string{
			"fly.toml",
		},
	}).File("fly.toml")
}

// Docker client with private registry
func Auth(ctx context.Context) {
	defer handleErr()

	d := daggerClient(ctx)
	defer d.Close()

	githubRef := os.Getenv("GITHUB_REF_NAME")
	if githubRef == "main" {
		flyctl := flyctlWithDockerConfig(ctx, d)
		authDocker(ctx, d, flyctl)
	} else {
		fmt.Println("\n🐳 Docker auth runs only in CI, main branch")
	}
}

func authDocker(ctx context.Context, d *dagger.Client, c *dagger.Container) {
	hostDockerConfigDir := os.Getenv("DOCKER_CONFIG")
	if hostDockerConfigDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			panic(readErr(err))
		}
		hostDockerConfigDir = filepath.Join(home, ".docker")
	}
	hostDockerClientConfig := filepath.Join(hostDockerConfigDir, "config.json")

	_, err := c.File(".docker/config.json").Export(ctx, hostDockerClientConfig)
	if err != nil {
		panic(createErr(err))
	}
}

// container image to private registry
func Publish(ctx context.Context) {
	defer handleErr()

	d := daggerClient(ctx)
	defer d.Close()

	publish(ctx, d)
}

func publish(ctx context.Context, d *dagger.Client) string {
	binary := build(ctx, d)

	githubRef := os.Getenv("GITHUB_REF_NAME")
	if githubRef == "main" {
		return publishImage(ctx, d, binary)
	} else {
		fmt.Println("\n📦 Publishing runs only in CI, main branch")
	}

	return ""
}

// zero-downtime deploy container image
func Deploy(ctx context.Context) {
	defer handleErr()

	d := daggerClient(ctx)
	defer d.Close()

	imageRef, err := hostEnv(ctx, d.Host(), "IMAGE_REF").Value(ctx)
	if err != nil {
		panic(misconfigureErr(err))
	}

	deploy(ctx, d, imageRef)
}

func deploy(ctx context.Context, d *dagger.Client, imageRef string) {
	githubRef := os.Getenv("GITHUB_REF_NAME")
	if githubRef == "main" {
		imageRefFlyValid, err := reference.ParseDockerRef(imageRef)
		if err != nil {
			panic(misconfigureErr(err))
		}

		flyctl := flyctlWithDockerConfig(ctx, d).
			WithExec([]string{
				"regions",
				"set", Amsterdam, Chicago, Paris, Singapore, Sunnyvale,
			}).
			WithExec([]string{
				"scale",
				"count", InstancesToDeploy,
				"--max-per-region", MaxInstancesPerRegion,
			}).
			WithExec([]string{
				"deploy",
				"--image", imageRefFlyValid.String(),
				"--strategy", DeployStrategy,
			})

		exitCode, err := flyctl.ExitCode(ctx)
		if err != nil {
			panic(unavailableErr(err))
		}
		if exitCode != 0 {
			panic(Exit{Code: exitCode, Error: err})
		}
	} else {
		fmt.Println("\n🎁 Deploying runs only in CI, main branch")
	}
}

// [lints, tests, auths], builds, publishes & deploys a new version of the app
func All(ctx context.Context) {
	// TODO: re-use the same client, run in parallel with err.Go
	mg.CtxDeps(ctx, Lint, Test, Auth)

	defer handleErr()

	d := daggerClient(ctx)
	defer d.Close()

	imageRef := publish(ctx, d)
	deploy(ctx, d, imageRef)
}

// stream app logs
func Logs(ctx context.Context) {
	defer handleErr()

	d := daggerClient(ctx)
	defer d.Close()

	// This command does not return,
	// therefore it will never be cached,
	// and it can be run multiple times
	_, err := flyctlWithDockerConfig(ctx, d).
		WithExec([]string{"logs"}).
		Stdout(ctx)

	if err != nil {
		panic(unavailableErr(err))
	}
}

func daggerClient(ctx context.Context) *dagger.Client {
	client, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stderr))
	if err != nil {
		panic(unavailableErr(err))
	}
	return client
}

func publishImage(ctx context.Context, d *dagger.Client, binary *dagger.File) string {
	ref := fmt.Sprintf("%s:%s", imageName(), gitSHA())

	refWithSHA, err := d.Container().
		From(fmt.Sprintf("alpine:%s", alpineVersion)).
		WithFile(fmt.Sprintf("/%s", binaryName), binary).
		WithNewFile("/GIT_SHA", dagger.ContainerWithNewFileOpts{
			Contents:    gitSHA(),
			Permissions: 444,
		}).
		WithNewFile("/GIT_AUTHOR", dagger.ContainerWithNewFileOpts{
			Contents:    author(),
			Permissions: 444,
		}).
		WithNewFile("/BUILD_URL", dagger.ContainerWithNewFileOpts{
			Contents:    buildURL(),
			Permissions: 444,
		}).
		WithFile("/fly.toml", deployConfig(d), dagger.ContainerWithFileOpts{
			Permissions: 444,
		}).
		WithEntrypoint([]string{fmt.Sprintf("/%s", binaryName)}).
		Publish(ctx, ref)

	if err != nil {
		panic(unavailableErr(err))
	}

	return refWithSHA
}

func gitSHA() string {
	gitSHA := os.Getenv("GITHUB_SHA")
	if gitSHA == "" {
		if gitHEAD, err := sh.Output("git", "rev-parse", "HEAD"); err == nil {
			gitSHA = fmt.Sprintf("%s.", gitHEAD)
		}
		gitSHA = fmt.Sprintf("%sdev", gitSHA)
	}

	return gitSHA
}

func author() string {
	author := os.Getenv("GITHUB_AUTHOR")
	if author == "" {
		author = os.Getenv("USER")
	}

	return author
}

func buildURL() string {
	githubServerURL := os.Getenv("GITHUB_SERVER_URL")
	githubRepository := os.Getenv("GITHUB_REPOSITORY")
	githubRunID := os.Getenv("GITHUB_RUN_ID")
	buildURL := fmt.Sprintf("%s/%s/actions/runs/%s", githubServerURL, githubRepository, githubRunID)

	if githubRunID == "" {
		if hostname, err := os.Hostname(); err == nil {
			buildURL = hostname
		}
		if cwd, err := os.Getwd(); err == nil {
			buildURL = fmt.Sprintf("%s:%s", buildURL, cwd)
		}
	}

	return buildURL
}

func flyctlWithDockerConfig(ctx context.Context, d *dagger.Client) *dagger.Container {
	flyToken := hostEnv(ctx, d.Host(), "FLY_API_TOKEN").Secret()

	flyctl := d.Container().
		From(fmt.Sprintf("flyio/flyctl:v%s", flyctlVersion)).
		WithSecretVariable("FLY_API_TOKEN", flyToken).
		WithMountedFile("fly.toml", flyConfig(d)).
		WithExec([]string{"auth", "docker"})

	exitCode, err := flyctl.ExitCode(ctx)

	if err != nil {
		panic(createErr(err))
	}

	if exitCode != 0 {
		panic(createErr(errors.New("Failed to add registry.fly.io as a Docker authenticated registry")))
	}

	return flyctl
}

func flyConfig(d *dagger.Client) *dagger.File {
	return d.Host().Directory(".").File("fly.toml")
}

func imageName() string {
	envImageURL := os.Getenv("IMAGE_URL")
	if envImageURL != "" {
		return envImageURL
	}

	return _imageName
}

func hostEnv(ctx context.Context, host *dagger.Host, varName string) *dagger.HostVariable {
	hostEnv := host.EnvVariable(varName)
	hostEnvVal, err := hostEnv.Value(ctx)
	if err != nil {
		panic(readErr(err))
	}
	if hostEnvVal == "" {
		panic(misconfigureErr(errors.New(fmt.Sprintf("💥 env var %s must be set\n", varName))))
	}
	return hostEnv
}

type Exit struct {
	Code  int
	Error error
}

func handleErr() {
	if e := recover(); e != nil {
		if exit, ok := e.(Exit); ok == true {
			fmt.Fprintf(os.Stderr, "%s\nsysexits(3) error code %d\n", exit.Error.Error(), exit.Code)
			os.Exit(exit.Code)
		}
		panic(e) // not an Exit, pass-through
	}
}

// https://man.openbsd.org/sysexits
func readErr(err error) Exit {
	return Exit{Code: 65, Error: err}
}

func unavailableErr(err error) Exit {
	return Exit{Code: 69, Error: err}
}

func createErr(err error) Exit {
	return Exit{Code: 73, Error: err}
}

func misconfigureErr(err error) Exit {
	return Exit{Code: 78, Error: err}
}

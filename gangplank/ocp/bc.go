/*
	Main interface into OCP Build targets.

	This supports running via:
	- generic Pod with a Service Account
	- an OpenShift buildConfig

*/

package ocp

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coreos/gangplank/cosa"
	"github.com/coreos/gangplank/spec"
	buildapiv1 "github.com/openshift/api/build/v1"
	log "github.com/sirupsen/logrus"
)

var (
	// srvBucket is the name of the bucket to use for remote
	// files being served up
	srvBucket = "source"

	// buildConfig is a builder.
	_ Builder = &buildConfig{}
)

func init() {
	buildJSONCodec = buildCodecFactory.LegacyCodec(buildapiv1.SchemeGroupVersion)
}

// buildConfig represent the input into a buildConfig.
type buildConfig struct {
	JobSpecURL  string `envVar:"COSA_JOBSPEC_URL"`
	JobSpecRef  string `envVar:"COSA_JOBSPEC_REF"`
	JobSpecFile string `envVar:"COSA_JOBSPEC_FILE"`
	CosaCmds    string `envVar:"COSA_CMDS"`

	// Information about the parent pod
	PodName      string `envVar:"COSA_POD_NAME"`
	PodIP        string `envVar:"COSA_POD_IP"`
	PodNameSpace string `envVar:"COSA_POD_NAMESPACE"`

	// HostIP is the kubernetes IP address of the running pod.
	HostIP  string
	HostPod string

	// Internal copy of the JobSpec
	JobSpec spec.JobSpec

	ClusterCtx ClusterContext
}

// newBC accepts a context and returns a buildConfig
func newBC(ctx context.Context, c *Cluster) (*buildConfig, error) {
	var v buildConfig
	rv := reflect.TypeOf(v)
	for i := 0; i < rv.NumField(); i++ {
		tag := rv.Field(i).Tag.Get(ocpStructTag)
		if tag == "" {
			continue
		}
		ev, found := os.LookupEnv(tag)
		if found {
			reflect.ValueOf(&v).Elem().Field(i).SetString(ev)
		}
	}

	// Init the OpenShift Build API Client.
	if err := ocpBuildClient(); err != nil {
		log.WithError(err).Error("Failed to initalized the OpenShift Build API Client")
		return nil, err
	}

	// Add the ClusterContext to the BuildConfig
	v.ClusterCtx = NewClusterContext(ctx, *c.toKubernetesCluster())
	ac, ns, kubeErr := GetClient(v.ClusterCtx)
	if kubeErr != nil {
		log.WithError(kubeErr).Info("Running without a cluster client")
	}

	if kubeErr != nil && ac != nil {
		v.HostPod = fmt.Sprintf("%s-%s-build",
			apiBuild.Annotations[buildapiv1.BuildConfigAnnotation],
			apiBuild.Annotations[buildapiv1.BuildNumberAnnotation],
		)

		_, ok := apiBuild.Annotations[podBuildRunnerTag]
		if ok {
			v.HostIP = apiBuild.Annotations[fmt.Sprintf(podBuildAnnotation, "IP")]
		} else {
			log.Info("Querying for pod ID")
			hIP, err := getPodIP(ac, ns, v.HostPod)
			if err != nil {
				log.Errorf("Failed to determine buildconfig's pod")
			}
			v.HostIP = hIP
		}

		log.WithFields(log.Fields{
			"buildconfig/name":   apiBuild.Annotations[buildapiv1.BuildConfigAnnotation],
			"buildconfig/number": apiBuild.Annotations[buildapiv1.BuildNumberAnnotation],
			"podname":            v.HostPod,
			"podIP":              v.HostIP,
		}).Info("found build.openshift.io/buildconfig identity")
	}

	if _, err := os.Stat(cosaSrvDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("Context dir %q does not exist", cosaSrvDir)
	}

	if err := os.Chdir(cosaSrvDir); err != nil {
		return nil, fmt.Errorf("Failed to switch to context dir: %s: %v", cosaSrvDir, err)
	}

	// Locate the jobspec from local input OR from a remote repo.
	jsF := spec.DefaultJobSpecFile
	if v.JobSpecFile != "" {
		jsF = v.JobSpecFile
	}
	v.JobSpecFile = jsF
	jsF = filepath.Join(cosaSrvDir, jsF)
	js, err := spec.JobSpecFromFile(jsF)
	if err != nil {
		v.JobSpec = js
	} else {
		njs, err := spec.JobSpecFromRepo(v.JobSpecURL, v.JobSpecFile, filepath.Base(jsF))
		if err != nil {
			v.JobSpec = njs
		}
	}

	log.Info("Running Pod in buildconfig mode.")
	return &v, nil
}

// Exec executes the command using the closure for the commands
func (bc *buildConfig) Exec(ctx ClusterContext) (err error) {
	curD, _ := os.Getwd()
	defer func(c string) { _ = os.Chdir(c) }(curD)

	if err := os.Chdir(cosaSrvDir); err != nil {
		return err
	}

	// Define, but do not start minio.
	m := newMinioServer()
	m.dir = cosaSrvDir
	m.Host = bc.HostIP

	// returnTo informs the workers where to send their bits
	returnTo := &Return{
		Minio:  m,
		Bucket: "builds",
	}

	// Prepare the remote files.
	var remoteFiles []*RemoteFile
	r, err := bc.ocpBinaryInput(m)
	if err != nil {
		return fmt.Errorf("failed to process binary input: %w", err)
	}
	remoteFiles = append(remoteFiles, r...)
	defer func() { _ = os.RemoveAll(filepath.Join(cosaSrvDir, sourceSubPath)) }()

	// Ensure that "builds/builds.json" is alway present if it exists.
	buildsJSON := filepath.Join(cosaSrvDir, "builds", "builds.json")
	if _, err := os.Stat(buildsJSON); err == nil {
		remoteFiles = append(
			remoteFiles,
			&RemoteFile{
				Bucket: "builds",
				Object: "builds.json",
				Minio:  m,
			})
	}

	// Discover the stages and render each command into a script.
	r, err = bc.discoverStages(m)
	if err != nil {
		return fmt.Errorf("failed to discover stages: %w", err)
	}
	remoteFiles = append(remoteFiles, r...)

	if len(bc.JobSpec.Stages) == 0 {
		log.Info(`
No work to do. Please define one of the following:
	- 'COSA_CMDS' envVar with the commands to execute
	- Jobspec stages in your JobSpec file
	- Provide files ending in .cosa.sh

File can be provided in the Git Tree or by the OpenShift
binary build interface.`)
		return nil
	}

	// Start minio after all the setup. Each directory is an implicit
	// bucket and files, are implicit keys.
	if err := m.start(ctx); err != nil {
		return fmt.Errorf("failed to start Minio: %w", err)
	}
	defer m.kill()

	if err := m.ensureBucketExists(ctx, "builds"); err != nil {
		return err
	}

	// Dump the jobspec
	log.Infof("Using JobSpec definition:")
	if err := bc.JobSpec.WriteYAML(log.New().Out); err != nil {
		return err
	}

	buildID := ""

	// Job control
	podCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	terminate := make(chan bool)
	errorCh := make(chan error)
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

		for {
			select {
			case err := <-errorCh:
				if err != nil {
					terminate <- true
				}
			case <-sig:
				terminate <- true
			case <-ctx.Done():
				terminate <- true
			case <-terminate:
				log.Info("Termination signaled")
				return
			}
		}
	}()

	type workFunction func(wg *sync.WaitGroup, errCh chan<- error)
	workerFuncs := make(map[int][]workFunction)

	// Range over the stages and create workFunction, which is added to the
	// workerFuncs. Each workFunction is executed as a go routine that begins
	// work as soon as the `build_dependencies` are available.
	for idx, ss := range bc.JobSpec.Stages {

		// copy the stage to prevent corruption
		s, err := ss.DeepCopy()
		if err != nil {
			return err
		}

		l := log.WithFields(log.Fields{
			"stage":              s.ID,
			"required_artifacts": s.RequireArtifacts,
		})

		anonFunc := func(wg *sync.WaitGroup, errCh chan<- error) {
			defer wg.Done()

			// ready is a function to ensure that the depedencies are ready/avaiable
			ready := func(ws *workSpec) bool {
				// For _each_ stage, we need to check if a meta.json exists.
				// mBuild - *cosa.Build representing meta.json
				mBuild, _, _ := cosa.ReadBuild(cosaSrvDir, buildID, "")

				// The buildID may have been updated by worker pod.
				// Log the fact for propserity.
				if mBuild != nil && mBuild.BuildID != buildID {
					log.WithField("buildID", mBuild.BuildID).Info("Found new build ID")
					buildID = mBuild.BuildID
					l = log.WithField("buildID", buildID)
					l.Info("Found new build ID")
				}

				buildPath := filepath.Join(buildID, cosa.BuilderArch())

				// Include any *json file
				jsonPath := filepath.Join(cosaSrvDir, "builds", buildPath)
				_ = filepath.Walk(jsonPath, func(path string, info os.FileInfo, err error) error {
					if info == nil {
						return nil
					}
					n := filepath.Base(info.Name())
					if !(strings.HasPrefix(n, "meta") && strings.HasSuffix(n, ".json")) {
						l.WithField("file", n).Warning("excluded")
						return nil
					}
					keyPath := filepath.Join(buildPath, n)
					ws.RemoteFiles = append(
						ws.RemoteFiles,
						&RemoteFile{
							Bucket: "builds",
							Minio:  m,
							Object: keyPath,
						},
					)
					l.WithField("file", keyPath).Info("Included metadata")
					return nil
				})

				foundCount := 0
				for _, artifact := range s.RequireArtifacts {
					l.WithField("artifact", artifact).Info("Checking for required artifact")
					if mBuild == nil {
						l.WithField("artifact", artifact).Info("meta.json is not available yet")
						return false
					}
					bArtifact, err := mBuild.GetArtifact(artifact)
					if err != nil {
						l.WithField("artifact", artifact).Info("artifacts is not available yet")
						return false
					}

					// get the Minio relative path for the object
					// the full path needs to be broken in to <BUILDID>/<ARCH>/<FILE>
					keyPath := filepath.Join(buildPath, filepath.Base(bArtifact.Path))
					l.WithField("path", keyPath).Info("Found required artifact")
					r := &RemoteFile{
						Artifact: bArtifact,
						Bucket:   "builds",
						Minio:    m,
						Object:   keyPath,
					}
					ws.RemoteFiles = append(ws.RemoteFiles, r)
					foundCount++
				}
				if len(s.RequireArtifacts) == foundCount {
					l.Infof("All dependencies for stage have been meet")
					return true
				}
				return false
			}

			// Loop until either until we get the termination signal or the
			// dependencies are met.
			for {

				select {
				case <-terminate:
					return
				default:
					ws := &workSpec{
						APIBuild:      apiBuild,
						ExecuteStages: []string{s.ID},
						JobSpec:       bc.JobSpec,
						RemoteFiles:   remoteFiles,
						Return:        returnTo,
					}

					if !ready(ws) {
						l.Warning("Waiting for dependencies")
						time.Sleep(15 * time.Second)
						break
					}

					l.Info("Worker dependences have been defined")
					eVars, err := ws.getEnvVars()
					if err != nil {
						errCh <- err
						return
					}

					cpod, err := NewCosaPodder(podCtx, apiBuild, idx)
					if err != nil {
						log.WithError(err).Error("Failed to create pod definition")
						errCh <- err
						return
					}

					l.Info("Executing worker pod")
					if err := cpod.WorkerRunner(podCtx, eVars); err != nil {
						log.WithError(err).Error("Failed stage execution")
						errCh <- err
						return
					}
				}
			}
		}

		l.Info("workerfunction defined and assinged")
		workerFuncs[s.ExecutionOrder] = append(workerFuncs[s.ExecutionOrder], anonFunc)
	}

	// iterate through each group of workerFuncs
	for key, wf := range workerFuncs {
		log.WithField("execution group", key).Info("Starting Group of workers")
		select {
		case <-terminate:
			break
		default:
			wg := &sync.WaitGroup{}
			for _, f := range wf {
				wg.Add(1)
				go f(wg, errorCh)
			}
			wg.Wait()
		}
	}

	// Yeah, this is lazy...
	args := []string{"find", "/srv", "-type", "f"}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func copyFile(src, dest string) error {
	srcF, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcF.Close()

	destF, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer destF.Close()

	if _, err := io.Copy(destF, srcF); err != nil {
		return err
	}
	return err
}

// discoverStages supports the envVar and *.cosa.sh scripts as implied stages.
// The envVar stage will be run first, followed by the `*.cosa.sh` scripts.
func (bc *buildConfig) discoverStages(m *minioServer) ([]*RemoteFile, error) {
	var remoteFiles []*RemoteFile

	if bc.JobSpec.Job.StrictMode {
		log.Info("Job strict mode is set, skipping automated stage discovery.")
		return nil, nil
	}
	log.Info("Strict mode is off: envVars and *.cosa.sh files are implied stages.")

	sPrefix := "/bin/bash -xeu -o pipefail %s"
	// Add the envVar commands
	if bc.CosaCmds != "" {
		bc.JobSpec.Stages = append(
			bc.JobSpec.Stages,
			spec.Stage{
				Description: "envVar defined commands",
				DirectExec:  true,
				Commands: []string{
					fmt.Sprintf(sPrefix, bc.CosaCmds),
				},
				ID: "envVar",
			},
		)
	}

	// Add discovered *.cosa.sh scripts into a single stage.
	// *.cosa.sh scripts are all run on the same worker pod.
	scripts := []string{}
	foundScripts, _ := filepath.Glob("*.cosa.sh")
	for _, s := range foundScripts {
		dn := filepath.Base(s)
		destPath := filepath.Join(cosaSrvDir, srvBucket, dn)
		if err := copyFile(s, destPath); err != nil {
			return remoteFiles, err
		}

		// We _could_ embed the scripts directly into the jobspec's stage
		// but the jospec is embedded as a envVar. To avoid runing into the
		// 32K character limit and we have an object store running, we'll just use
		// that.
		remoteFiles = append(
			remoteFiles,
			&RemoteFile{
				Bucket: srvBucket,
				Object: dn,
				Minio:  m,
			},
		)

		// Add the script to the command interface.
		scripts = append(
			scripts,
			fmt.Sprintf(sPrefix, filepath.Join(cosaSrvDir, srvBucket, dn)),
		)
	}
	if len(scripts) > 0 {
		bc.JobSpec.Stages = append(
			bc.JobSpec.Stages,
			spec.Stage{
				Description: "*.cosa.sh scripts",
				DirectExec:  true,
				Commands:    scripts,
				ID:          "cosa.sh",
			},
		)
	}
	return remoteFiles, nil
}

// ocpBinaryInput decompresses the binary input. If the binary input is a tarball
// with an embedded JobSpec, its extracted, read and used.
func (bc *buildConfig) ocpBinaryInput(m *minioServer) ([]*RemoteFile, error) {
	var remoteFiles []*RemoteFile
	bin, err := recieveInputBinary()
	if err != nil {
		return nil, err
	}
	if bin == "" {
		return nil, nil
	}

	if strings.HasSuffix(bin, "source.bin") {
		f, err := os.Open(bin)
		if err != nil {
			return nil, err
		}

		if err := decompress(f, cosaSrvDir); err != nil {
			return nil, err
		}
		dir, key := filepath.Split(bin)
		bucket := filepath.Base(dir)
		r := &RemoteFile{
			Bucket:     bucket,
			Object:     key,
			Minio:      m,
			Compressed: true,
		}
		remoteFiles = append(remoteFiles, r)
		log.Info("Binary input will be served to remote mos.")
	}

	// Look for a jobspec in the binary payload.
	jsFile := ""
	candidateSpec := filepath.Join(cosaSrvDir, bc.JobSpecFile)
	_, err = os.Stat(candidateSpec)
	if err == nil {
		log.Info("Found jobspec file in binary payload.")
		jsFile = candidateSpec
	}

	// Treat any yaml files as jobspec's.
	if strings.HasSuffix(apiBuild.Spec.Source.Binary.AsFile, "yaml") {
		jsFile = bin
	}

	// Load the JobSpecFile
	if jsFile != "" {
		log.WithField("jobspec", bin).Info("treating source as a jobspec")
		js, err := spec.JobSpecFromFile(jsFile)
		if err != nil {
			return nil, err
		}
		log.Info("Using OpenShift provided JobSpec")
		bc.JobSpec = js

		if bc.JobSpec.Recipe.GitURL != "" {
			log.Info("Jobpsec references a git repo -- ignoring buildconfig reference")
			apiBuild.Spec.Source.Git = new(buildapiv1.GitBuildSource)
			apiBuild.Spec.Source.Git.URI = bc.JobSpec.Recipe.GitURL
			apiBuild.Spec.Source.Git.Ref = bc.JobSpec.Recipe.GitRef
		}
	}
	return remoteFiles, nil
}

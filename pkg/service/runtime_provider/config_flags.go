/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package runtime_provider

import (
	"github.com/spf13/pflag"
	"helm.sh/helm/pkg/action"
	"helm.sh/helm/pkg/cli"
	"helm.sh/helm/pkg/kube"
	"helm.sh/helm/pkg/storage"
	"helm.sh/helm/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

const (
	flagClusterName      = "cluster"
	flagAuthInfoName     = "user"
	flagContext          = "context"
	flagNamespace        = "namespace"
	flagAPIServer        = "server"
	flagInsecure         = "insecure-skip-tls-verify"
	flagCertFile         = "client-certificate"
	flagKeyFile          = "client-key"
	flagCAFile           = "certificate-authority"
	flagBearerToken      = "token"
	flagImpersonate      = "as"
	flagImpersonateGroup = "as-group"
	flagUsername         = "username"
	flagPassword         = "password"
	flagTimeout          = "request-timeout"
	flagHTTPCacheDir     = "cache-dir"
)

var defaultCacheDir = filepath.Join(homedir.HomeDir(), ".kube", "http-cache")
var (
	settings   cli.EnvSettings
	config     genericclioptions.RESTClientGetter
	configOnce sync.Once
)

var _ genericclioptions.RESTClientGetter = &ConfigFlags{}

// ConfigFlags composes the set of values necessary
// for obtaining a REST client config
type ConfigFlags struct {
	CacheDir   *string
	KubeConfig *string

	// config flags
	ClusterName      *string
	AuthInfoName     *string
	Context          *string
	Namespace        *string
	APIServer        *string
	Insecure         *bool
	CertFile         *string
	KeyFile          *string
	CAFile           *string
	BearerToken      *string
	Impersonate      *string
	ImpersonateGroup *[]string
	Username         *string
	Password         *string
	Timeout          *string

	clientConfig clientcmd.ClientConfig
	lock         sync.Mutex
	// If set to true, will use persistent client config and
	// propagate the config to the places that need it, rather than
	// loading the config multiple times
	usePersistentConfig bool
	CredentialContent   []byte
}

// ToRESTConfig implements RESTClientGetter.
// Returns a REST client configuration based on a provided path
// to a .kubeconfig file, loading rules, and config flag overrides.
// Expects the AddFlags method to have been called.
func (f *ConfigFlags) ToRESTConfig() (*rest.Config, error) {
	return f.ToRawKubeConfigLoader().ClientConfig()
}

// ToRawKubeConfigLoader binds config flag values to config overrides
// Returns an interactive clientConfig if the password flag is enabled,
// or a non-interactive clientConfig otherwise.
func (f *ConfigFlags) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	if f.usePersistentConfig {
		return f.toRawKubePersistentConfigLoader()
	}
	return f.toRawKubeConfigLoader()
}

func (f *ConfigFlags) toRawKubeConfigLoader() clientcmd.ClientConfig {
	var clientConfig clientcmd.ClientConfig

	clientConfig, _ = clientcmd.NewClientConfigFromBytes([]byte{})

	// we only have an interactive prompt when a password is allowed
	//if f.Password == nil {
	//	clientConfig = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	//} else {
	//	clientConfig = clientcmd.NewInteractiveDeferredLoadingClientConfig(loadingRules, overrides, os.Stdin)
	//}

	return clientConfig
}

// toRawKubePersistentConfigLoader binds config flag values to config overrides
// Returns a persistent clientConfig for propagation.
func (f *ConfigFlags) toRawKubePersistentConfigLoader() clientcmd.ClientConfig {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.clientConfig == nil {
		f.clientConfig = f.toRawKubeConfigLoader()
	}

	return f.clientConfig
}

// ToDiscoveryClient implements RESTClientGetter.
// Expects the AddFlags method to have been called.
// Returns a CachedDiscoveryInterface using a computed RESTConfig.
func (f *ConfigFlags) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	//config, err := f.ToRESTConfig()
	//if err != nil {
	//	return nil, err
	//}
	//
	//// The more groups you have, the more discovery requests you need to make.
	//// given 25 groups (our groups + a few custom resources) with one-ish version each, discovery needs to make 50 requests
	//// double it just so we don't end up here again for a while.  This config is only used for discovery.
	//config.Burst = 100
	//
	//// retrieve a user-provided value for the "cache-dir"
	//// defaulting to ~/.kube/http-cache if no user-value is given.
	//httpCacheDir := defaultCacheDir
	//if f.CacheDir != nil {
	//	httpCacheDir = *f.CacheDir
	//}
	//
	//discoveryCacheDir := computeDiscoverCacheDir(filepath.Join(homedir.HomeDir(), ".kube", "cache", "discovery"), config.Host)
	//return diskcached.NewCachedDiscoveryClientForConfig(config, discoveryCacheDir, httpCacheDir, time.Duration(10*time.Minute))
	return nil, nil
}

// ToRESTMapper returns a mapper.
func (f *ConfigFlags) ToRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := f.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
	expander := restmapper.NewShortcutExpander(mapper, discoveryClient)
	return expander, nil
}

// AddFlags binds client configuration flags to a given flagset
func (f *ConfigFlags) AddFlags(flags *pflag.FlagSet) {
	if f.KubeConfig != nil {
		flags.StringVar(f.KubeConfig, "kubeconfig", *f.KubeConfig, "Path to the kubeconfig file to use for CLI requests.")
	}
	if f.CacheDir != nil {
		flags.StringVar(f.CacheDir, flagHTTPCacheDir, *f.CacheDir, "Default HTTP cache directory")
	}

	// add config options
	if f.CertFile != nil {
		flags.StringVar(f.CertFile, flagCertFile, *f.CertFile, "Path to a client certificate file for TLS")
	}
	if f.KeyFile != nil {
		flags.StringVar(f.KeyFile, flagKeyFile, *f.KeyFile, "Path to a client key file for TLS")
	}
	if f.BearerToken != nil {
		flags.StringVar(f.BearerToken, flagBearerToken, *f.BearerToken, "Bearer token for authentication to the API server")
	}
	if f.Impersonate != nil {
		flags.StringVar(f.Impersonate, flagImpersonate, *f.Impersonate, "Username to impersonate for the operation")
	}
	if f.ImpersonateGroup != nil {
		flags.StringArrayVar(f.ImpersonateGroup, flagImpersonateGroup, *f.ImpersonateGroup, "Group to impersonate for the operation, this flag can be repeated to specify multiple groups.")
	}
	if f.Username != nil {
		flags.StringVar(f.Username, flagUsername, *f.Username, "Username for basic authentication to the API server")
	}
	if f.Password != nil {
		flags.StringVar(f.Password, flagPassword, *f.Password, "Password for basic authentication to the API server")
	}
	if f.ClusterName != nil {
		flags.StringVar(f.ClusterName, flagClusterName, *f.ClusterName, "The name of the kubeconfig cluster to use")
	}
	if f.AuthInfoName != nil {
		flags.StringVar(f.AuthInfoName, flagAuthInfoName, *f.AuthInfoName, "The name of the kubeconfig user to use")
	}
	if f.Namespace != nil {
		flags.StringVarP(f.Namespace, flagNamespace, "n", *f.Namespace, "If present, the namespace scope for this CLI request")
	}
	if f.Context != nil {
		flags.StringVar(f.Context, flagContext, *f.Context, "The name of the kubeconfig context to use")
	}

	if f.APIServer != nil {
		flags.StringVarP(f.APIServer, flagAPIServer, "s", *f.APIServer, "The address and port of the Kubernetes API server")
	}
	if f.Insecure != nil {
		flags.BoolVar(f.Insecure, flagInsecure, *f.Insecure, "If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure")
	}
	if f.CAFile != nil {
		flags.StringVar(f.CAFile, flagCAFile, *f.CAFile, "Path to a cert file for the certificate authority")
	}
	if f.Timeout != nil {
		flags.StringVar(f.Timeout, flagTimeout, *f.Timeout, "The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests.")
	}

}

// NewConfigFlags returns ConfigFlags with default values set
func NewConfigFlags(usePersistentConfig bool, credentialContent []byte) *ConfigFlags {
	impersonateGroup := []string{}
	insecure := false

	return &ConfigFlags{
		Insecure:   &insecure,
		Timeout:    stringptr("0"),
		KubeConfig: stringptr(""),

		CacheDir:         stringptr(defaultCacheDir),
		ClusterName:      stringptr(""),
		AuthInfoName:     stringptr(""),
		Context:          stringptr(""),
		Namespace:        stringptr(""),
		APIServer:        stringptr(""),
		CertFile:         stringptr(""),
		KeyFile:          stringptr(""),
		CAFile:           stringptr(""),
		BearerToken:      stringptr(""),
		Impersonate:      stringptr(""),
		ImpersonateGroup: &impersonateGroup,

		usePersistentConfig: usePersistentConfig,
		CredentialContent:   credentialContent,
	}
}

func stringptr(val string) *string {
	return &val
}

// overlyCautiousIllegalFileCharacters matches characters that *might* not be supported.  Windows is really restrictive, so this is really restrictive
var overlyCautiousIllegalFileCharacters = regexp.MustCompile(`[^(\w/\.)]`)

// computeDiscoverCacheDir takes the parentDir and the host and comes up with a "usually non-colliding" name.
func computeDiscoverCacheDir(parentDir, host string) string {
	// strip the optional scheme from host if its there:
	schemelessHost := strings.Replace(strings.Replace(host, "https://", "", 1), "http://", "", 1)
	// now do a simple collapse of non-AZ09 characters.  Collisions are possible but unlikely.  Even if we do collide the problem is short lived
	safeHost := overlyCautiousIllegalFileCharacters.ReplaceAllString(schemelessHost, "_")
	return filepath.Join(parentDir, safeHost)
}

func kubeConfig(credentialContent []byte) genericclioptions.RESTClientGetter {
	configOnce.Do(func() {
		config = NewConfigFlags(false, credentialContent)
	})
	return config
}

func getNamespace(credentialContent []byte) string {
	if ns, _, err := kubeConfig(credentialContent).ToRawKubeConfigLoader().Namespace(); err == nil {
		return ns
	}
	return "default"
}

func NewActionConfig(allNamespaces bool, credentialContent []byte) *action.Configuration {
	kc := kube.New(kubeConfig(credentialContent))
	//kc.Log = logf

	clientset, err := kc.KubernetesClientSet()
	if err != nil {
		// TODO return error
		log.Fatal(err)
	}
	var namespace string
	if !allNamespaces {
		namespace = getNamespace(credentialContent)
	}

	var store *storage.Storage
	switch os.Getenv("HELM_DRIVER") {
	case "secret", "secrets", "":
		d := driver.NewSecrets(clientset.CoreV1().Secrets(namespace))
		//d.Log = logf
		store = storage.Init(d)
	case "configmap", "configmaps":
		d := driver.NewConfigMaps(clientset.CoreV1().ConfigMaps(namespace))
		//d.Log = logf
		store = storage.Init(d)
	case "memory":
		d := driver.NewMemory()
		store = storage.Init(d)
	default:
		// Not sure what to do here.
		panic("Unknown driver in HELM_DRIVER: " + os.Getenv("HELM_DRIVER"))
	}

	return &action.Configuration{
		RESTClientGetter: kubeConfig(credentialContent),
		KubeClient:       kc,
		Releases:         store,
		Log:              nil,
	}
}

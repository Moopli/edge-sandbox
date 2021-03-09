/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package startcmd

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/hyperledger/aries-framework-go-ext/component/vdr/trustbloc"
	vdrapi "github.com/hyperledger/aries-framework-go/pkg/framework/aries/api/vdr"
	"github.com/hyperledger/aries-framework-go/pkg/framework/context"
	vdrpkg "github.com/hyperledger/aries-framework-go/pkg/vdr"
	"github.com/hyperledger/aries-framework-go/pkg/vdr/httpbinding"
	"github.com/spf13/cobra"
	"github.com/trustbloc/edge-core/pkg/log"
	"github.com/trustbloc/edge-core/pkg/restapi/logspec"
	cmdutils "github.com/trustbloc/edge-core/pkg/utils/cmd"
	tlsutils "github.com/trustbloc/edge-core/pkg/utils/tls"

	"github.com/trustbloc/sandbox/cmd/common"
	"github.com/trustbloc/sandbox/pkg/restapi/acerp"
	"github.com/trustbloc/sandbox/pkg/restapi/acerp/operation"
)

const (
	hostURLFlagName      = "host-url"
	hostURLFlagShorthand = "u"
	hostURLFlagUsage     = "URL to run the rp instance on. Format: HostName:Port."
	hostURLEnvKey        = "ACE_HOST_URL"

	tlsCertFileFlagName  = "tls-cert-file"
	tlsCertFileFlagUsage = "tls certificate file." +
		" Alternatively, this can be set with the following environment variable: " + tlsCertFileEnvKey
	tlsCertFileEnvKey = "ACE_TLS_CERT_FILE"

	tlsKeyFileFlagName  = "tls-key-file"
	tlsKeyFileFlagUsage = "tls key file." +
		" Alternatively, this can be set with the following environment variable: " + tlsKeyFileEnvKey
	tlsKeyFileEnvKey = "ACE_TLS_KEY_FILE"

	tlsSystemCertPoolFlagName  = "tls-systemcertpool"
	tlsSystemCertPoolFlagUsage = "Use system certificate pool." +
		" Possible values [true] [false]. Defaults to false if not set." +
		" Alternatively, this can be set with the following environment variable: " + tlsSystemCertPoolEnvKey
	tlsSystemCertPoolEnvKey = "ACE_TLS_SYSTEMCERTPOOL"

	tlsCACertsFlagName  = "tls-cacerts"
	tlsCACertsFlagUsage = "Comma-Separated list of ca certs path." +
		" Alternatively, this can be set with the following environment variable: " + tlsCACertsEnvKey
	tlsCACertsEnvKey = "ACE_TLS_CACERTS"

	demoModeFlagName  = "demo-mode"
	demoModeFlagUsage = "Demo mode." +
		" Mandatory - Possible values [ucis] [cbp] [benefits]."
	demoModeEnvKey = "ACE_DEMO_MODE"

	// vault server url
	vaultServerURLFlagName  = "vault-server-url"
	vaultServerURLFlagUsage = "URL of the vault server. This field is mandatory."
	vaultServerURLEnvKey    = "ACE_VAULT_SERVER_URL"

	// comparator url
	comparatorURLFlagName  = "comparator-url"
	comparatorURLFlagUsage = "URL of the comparator. This field is mandatory."
	comparatorURLEnvKey    = "ACE_COMPARATOR_URL"

	// vc issuer server url
	vcIssuerURLFlagName  = "vc-issuer-url"
	vcIssuerURLFlagUsage = "URL of the VC Issuer service. This field is mandatory."
	vcIssuerURLEnvKey    = "ACE_VC_ISSUER_URL"

	requestTokensFlagName  = "request-tokens"
	requestTokensEnvKey    = "ACE_REQUEST_TOKENS" //nolint:gosec
	requestTokensFlagUsage = "Tokens used for http request " +
		" Alternatively, this can be set with the following environment variable: " + requestTokensEnvKey

	// host external url
	hostExternalURLFlagName  = "host-external-url"
	hostExternalURLFlagUsage = "Host External URL. This field is mandatory."
	hostExternalURLEnvKey    = "ACE_HOST_EXTERNAL_URL"

	// account link profile id
	accountLinkProfileFlagName  = "account-link-profile"
	accountLinkProfileFlagUsage = "Account Link Profile."
	accountLinkProfileEnvKey    = "ACE_ACCOUNT_LINK_PROFILE"

	// extractor profile id
	extractorProfileFlagName  = "extractor-profile"
	extractorProfileFlagUsage = "Extractor Profile."
	extractorProfileEnvKey    = "ACE_EXTRACTOR_PROFILE"

	// did resolver url
	didResolverURLFlagName  = "did-resolver-url"
	didResolverURLFlagUsage = "DID Resolver URL."
	didResolverURLEnvKey    = "ACE_DID_RESOLVER_URL"

	tokenLength2 = 2
)

// nolint:gochecknoglobals
var supportedModes = map[string]demoModeConf{
	"ucis":     {uiPath: "ucis_dept", svcName: "UCIS"},
	"cbp":      {uiPath: "cbp_dept", svcName: "CBP"},
	"benefits": {uiPath: "benefits_dept", svcName: "Benefits Settlement"},
}

var logger = log.New("ace-rp-rest")

type demoModeConf struct {
	uiPath  string
	svcName string
}

type server interface {
	ListenAndServe(host, certFile, keyFile string, router http.Handler) error
}

// HTTPServer represents an actual HTTP server implementation.
type HTTPServer struct{}

// ListenAndServe starts the server using the standard Go HTTP server implementation.
func (s *HTTPServer) ListenAndServe(host, certFile, keyFile string, router http.Handler) error {
	if certFile != "" && keyFile != "" {
		return http.ListenAndServeTLS(host, certFile, keyFile, router)
	}

	return http.ListenAndServe(host, router)
}

type rpParameters struct {
	srv                server
	hostURL            string
	hostExternalURL    string
	tlsCertFile        string
	tlsKeyFile         string
	tlsSystemCertPool  bool
	tlsCACerts         []string
	logLevel           string
	dbParams           *common.DBParameters
	modeConf           demoModeConf
	vaultServerURL     string
	comparatorURL      string
	vcIssuerURL        string
	accountLinkProfile string
	extractorProfile   string
	requestTokens      map[string]string
	didResolverURL     string
}

type tlsConfig struct {
	certFile       string
	keyFile        string
	systemCertPool bool
	caCerts        []string
}

// GetStartCmd returns the Cobra start command.
func GetStartCmd(srv server) *cobra.Command {
	startCmd := createStartCmd(srv)

	createFlags(startCmd)

	return startCmd
}

func createStartCmd(srv server) *cobra.Command { //nolint: funlen, gocyclo
	return &cobra.Command{
		Use:   "start",
		Short: "Start ACE RP",
		Long:  "Start Anonymous Comparator and Extractor (ACE) RP",
		RunE: func(cmd *cobra.Command, args []string) error {
			hostURL, err := cmdutils.GetUserSetVarFromString(cmd, hostURLFlagName, hostURLEnvKey, false)
			if err != nil {
				return err
			}

			dbParams, err := common.DBParams(cmd)
			if err != nil {
				return err
			}

			tlsConfg, err := getTLS(cmd)
			if err != nil {
				return err
			}

			loggingLevel, err := cmdutils.GetUserSetVarFromString(cmd, common.LogLevelFlagName, common.LogLevelEnvKey, true)
			if err != nil {
				return err
			}

			demoModeFlag, err := cmdutils.GetUserSetVarFromString(cmd, demoModeFlagName, demoModeEnvKey, false)
			if err != nil {
				return err
			}

			demoModeConf, ok := supportedModes[demoModeFlag]
			if !ok {
				return fmt.Errorf("invalid demo mode : %s", demoModeFlag)
			}

			vaultServerURL, err := cmdutils.GetUserSetVarFromString(cmd, vaultServerURLFlagName,
				vaultServerURLEnvKey, false)
			if err != nil {
				return err
			}

			vcIssuerURL, err := cmdutils.GetUserSetVarFromString(cmd, vcIssuerURLFlagName, vcIssuerURLEnvKey, false)
			if err != nil {
				return err
			}

			hostExternalURL, err := cmdutils.GetUserSetVarFromString(cmd, hostExternalURLFlagName, hostExternalURLEnvKey, false)
			if err != nil {
				return err
			}

			accountLinkProfile, err := cmdutils.GetUserSetVarFromString(cmd,
				accountLinkProfileFlagName, accountLinkProfileEnvKey, true)
			if err != nil {
				return err
			}

			requestTokens, err := getRequestTokens(cmd)
			if err != nil {
				return err
			}

			comparatorURL, err := cmdutils.GetUserSetVarFromString(cmd, comparatorURLFlagName,
				comparatorURLEnvKey, false)
			if err != nil {
				return err
			}

			extractorProfile, err := cmdutils.GetUserSetVarFromString(cmd,
				extractorProfileFlagName, extractorProfileEnvKey, true)
			if err != nil {
				return err
			}

			didResolverURL, err := cmdutils.GetUserSetVarFromString(cmd,
				didResolverURLFlagName, didResolverURLEnvKey, false)
			if err != nil {
				return err
			}

			parameters := &rpParameters{
				srv:                srv,
				hostURL:            strings.TrimSpace(hostURL),
				hostExternalURL:    hostExternalURL,
				tlsCertFile:        tlsConfg.certFile,
				tlsKeyFile:         tlsConfg.keyFile,
				tlsSystemCertPool:  tlsConfg.systemCertPool,
				tlsCACerts:         tlsConfg.caCerts,
				logLevel:           loggingLevel,
				dbParams:           dbParams,
				modeConf:           demoModeConf,
				vaultServerURL:     vaultServerURL,
				comparatorURL:      comparatorURL,
				vcIssuerURL:        vcIssuerURL,
				accountLinkProfile: accountLinkProfile,
				extractorProfile:   extractorProfile,
				requestTokens:      requestTokens,
				didResolverURL:     didResolverURL,
			}

			return startRP(parameters)
		},
	}
}

func getTLS(cmd *cobra.Command) (*tlsConfig, error) {
	tlsCertFile, err := cmdutils.GetUserSetVarFromString(cmd, tlsCertFileFlagName,
		tlsCertFileEnvKey, true)
	if err != nil {
		return nil, err
	}

	tlsKeyFile, err := cmdutils.GetUserSetVarFromString(cmd, tlsKeyFileFlagName,
		tlsKeyFileEnvKey, true)
	if err != nil {
		return nil, err
	}

	tlsSystemCertPoolString, err := cmdutils.GetUserSetVarFromString(cmd, tlsSystemCertPoolFlagName,
		tlsSystemCertPoolEnvKey, true)
	if err != nil {
		return nil, err
	}

	tlsSystemCertPool := false
	if tlsSystemCertPoolString != "" {
		tlsSystemCertPool, err = strconv.ParseBool(tlsSystemCertPoolString)
		if err != nil {
			return nil, err
		}
	}

	tlsCACerts, err := cmdutils.GetUserSetVarFromArrayString(cmd, tlsCACertsFlagName,
		tlsCACertsEnvKey, true)
	if err != nil {
		return nil, err
	}

	return &tlsConfig{certFile: tlsCertFile,
		keyFile: tlsKeyFile, systemCertPool: tlsSystemCertPool, caCerts: tlsCACerts}, nil
}

func createFlags(startCmd *cobra.Command) {
	common.Flags(startCmd)
	startCmd.Flags().StringP(hostURLFlagName, hostURLFlagShorthand, "", hostURLFlagUsage)
	startCmd.Flags().StringP(tlsCertFileFlagName, "", "", tlsCertFileFlagUsage)
	startCmd.Flags().StringP(tlsKeyFileFlagName, "", "", tlsKeyFileFlagUsage)
	startCmd.Flags().StringP(tlsSystemCertPoolFlagName, "", "",
		tlsSystemCertPoolFlagUsage)
	startCmd.Flags().StringArrayP(tlsCACertsFlagName, "", []string{}, tlsCACertsFlagUsage)
	startCmd.Flags().StringP(demoModeFlagName, "", "", demoModeFlagUsage)
	startCmd.Flags().StringP(vaultServerURLFlagName, "", "", vaultServerURLFlagUsage)
	startCmd.Flags().StringP(comparatorURLFlagName, "", "", comparatorURLFlagUsage)
	startCmd.Flags().StringP(vcIssuerURLFlagName, "", "", vcIssuerURLFlagUsage)
	startCmd.Flags().StringP(hostExternalURLFlagName, "", "", hostExternalURLFlagUsage)
	startCmd.Flags().StringP(accountLinkProfileFlagName, "", "", accountLinkProfileFlagUsage)
	startCmd.Flags().StringP(extractorProfileFlagName, "", "", extractorProfileFlagUsage)
	startCmd.Flags().StringP(didResolverURLFlagName, "", "", didResolverURLFlagUsage)
	startCmd.Flags().StringArrayP(requestTokensFlagName, "", []string{}, requestTokensFlagUsage)
	startCmd.Flags().StringP(common.LogLevelFlagName, common.LogLevelFlagShorthand, "", common.LogLevelPrefixFlagUsage)
}

func startRP(parameters *rpParameters) error {
	if parameters.logLevel != "" {
		common.SetDefaultLogLevel(logger, parameters.logLevel)
	}

	rootCAs, err := tlsutils.GetCertPool(parameters.tlsSystemCertPool, parameters.tlsCACerts)
	if err != nil {
		return err
	}

	tlsConfig := &tls.Config{RootCAs: rootCAs, MinVersion: tls.VersionTLS12}

	basePath := "static/" + parameters.modeConf.uiPath
	router := pathPrefix(basePath)

	storeProvider, err := common.InitEdgeStore(parameters.dbParams, logger)
	if err != nil {
		return err
	}

	vdri, err := createVDRI(parameters.didResolverURL, tlsConfig)
	if err != nil {
		return err
	}

	cfg := &operation.Config{
		StoreProvider:        storeProvider,
		HomePageHTML:         basePath + "/index.html",
		LoginHTML:            basePath + "/login.html",
		DashboardHTML:        basePath + "/dashboard.html",
		ConsentHTML:          basePath + "/consent.html",
		AccountLinkedHTML:    basePath + "/accountlinked.html",
		AccountNotLinkedHTML: basePath + "/accountnotlinked.html",
		TLSConfig:            tlsConfig,
		VaultServerURL:       parameters.vaultServerURL,
		ComparatorURL:        parameters.comparatorURL,
		VCIssuerURL:          parameters.vcIssuerURL,
		AccountLinkProfile:   parameters.accountLinkProfile,
		ExtractorProfile:     parameters.extractorProfile,
		HostExternalURL:      parameters.hostExternalURL,
		RequestTokens:        parameters.requestTokens,
		SvcName:              parameters.modeConf.svcName,
		VDRI:                 vdri,
	}

	aceRpService, err := acerp.New(cfg)
	if err != nil {
		return err
	}

	handlers := aceRpService.GetOperations()

	for _, handler := range handlers {
		router.HandleFunc(handler.Path(), handler.Handle()).Methods(handler.Method())
	}

	for _, handler := range logspec.New().GetOperations() {
		router.HandleFunc(handler.Path(), handler.Handle()).Methods(handler.Method())
	}

	return parameters.srv.ListenAndServe(parameters.hostURL, parameters.tlsCertFile, parameters.tlsKeyFile, router)
}

func pathPrefix(path string) *mux.Router {
	router := mux.NewRouter()

	fs := http.FileServer(http.Dir(path))
	router.Handle("/", fs)
	router.PathPrefix("/img/").Handler(fs)
	router.PathPrefix("/internal/img/").Handler(fs)
	router.PathPrefix("/showregister").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, path+"/register.html")
	})
	router.PathPrefix("/internal").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, path+"/portal.html")
	})

	return router
}

func getRequestTokens(cmd *cobra.Command) (map[string]string, error) {
	requestTokens, err := cmdutils.GetUserSetVarFromArrayString(cmd, requestTokensFlagName,
		requestTokensEnvKey, true)
	if err != nil {
		return nil, err
	}

	tokens := make(map[string]string)

	for _, token := range requestTokens {
		split := strings.Split(token, "=")
		switch len(split) {
		case tokenLength2:
			tokens[split[0]] = split[1]
		default:
			logger.Warnf("invalid token '%s'", token)
		}
	}

	return tokens, nil
}

func createVDRI(didResolverURL string, tlsConfig *tls.Config) (vdrapi.Registry, error) {
	didResolverVDRI, err := httpbinding.New(didResolverURL,
		httpbinding.WithAccept(func(method string) bool {
			return method == "v1" || method == "elem" || method == "sov" ||
				method == "web" || method == "key" || method == "factom"
		}))
	if err != nil {
		return nil, fmt.Errorf("failed to create new universal resolver vdr: %w", err)
	}

	vdrProvider, err := context.New(context.WithKMS(nil))
	if err != nil {
		return nil, fmt.Errorf("failed to create new vdr provider: %w", err)
	}

	blocVDR, err := trustbloc.New(nil,
		trustbloc.WithResolverURL(didResolverURL),
		trustbloc.WithTLSConfig(tlsConfig),
	)
	if err != nil {
		return nil, err
	}

	return vdrpkg.New(vdrProvider, vdrpkg.WithVDR(blocVDR), vdrpkg.WithVDR(didResolverVDRI)), nil
}

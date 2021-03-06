/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configless

import (
	"io/ioutil"
	"strings"

	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/fab"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/msp"
	"github.com/hyperledger/fabric-sdk-go/pkg/util/pathvar"
	"github.com/pkg/errors"
)

// identityconfig_override_test.go is an example of programmatically configuring the sdk by injecting instances that implement IdentityConfig's functions (representing the sdk's msp configs)
// for the sake of overriding IdentityConfig integration tests, the structure variables below are similar to what is found in /test/fixtures/config/config_test.yaml
// application developers can fully override these functions to load configs in any way that suit their application need

var (

	// creating instances of each interface to be referenced in the integration tests:
	clientImpl              = &exampleClient{}
	caConfigImpl            = &exampleCaConfig{}
	caServerCertsImpl       = &exampleCaServerCerts{}
	caClientKeyImpl         = &exampleCaClientKey{}
	caClientCertImpl        = &exampleCaClientCert{}
	caKeyStorePathImpl      = &exampleCaKeyStorePath{}
	credentialStorePathImpl = &exampleCredentialStorePath{}

	identityConfigImpls = []interface{}{
		clientImpl,
		caConfigImpl,
		caServerCertsImpl,
		caClientKeyImpl,
		caClientCertImpl,
		caKeyStorePathImpl,
		credentialStorePathImpl,
	}
)

type exampleClient struct {
}

func (m *exampleClient) Client() (*msp.ClientConfig, error) {
	client := networkConfig.Client

	client.Organization = strings.ToLower(client.Organization)
	client.TLSCerts.Path = pathvar.Subst(client.TLSCerts.Path)
	client.TLSCerts.Client.Key.Path = pathvar.Subst(client.TLSCerts.Client.Key.Path)
	client.TLSCerts.Client.Cert.Path = pathvar.Subst(client.TLSCerts.Client.Cert.Path)

	return &client, nil
}

type exampleCaConfig struct{}

func (m *exampleCaConfig) CAConfig(org string) (*msp.CAConfig, error) {
	return getCAConfig(&networkConfig, org)
}

// the below function is used in multiple implementations, this is fine because networkConfig is the same for all of them
func getCAConfig(networkConfig *fab.NetworkConfig, org string) (*msp.CAConfig, error) {
	if len(networkConfig.Organizations[strings.ToLower(org)].CertificateAuthorities) == 0 {
		return nil, errors.Errorf("organization %s has no Certificate Authorities setup. Make sure each org has at least 1 configured", org)
	}
	//for now, we're only loading the first Cert Authority by default. TODO add logic to support passing the Cert Authority ID needed by the client.
	certAuthorityName := networkConfig.Organizations[strings.ToLower(org)].CertificateAuthorities[0]

	if certAuthorityName == "" {
		return nil, errors.Errorf("certificate authority empty for %s. Make sure each org has at least 1 non empty certificate authority name", org)
	}

	caConfig, ok := networkConfig.CertificateAuthorities[strings.ToLower(certAuthorityName)]
	if !ok {
		// EntityMatchers are not supported in this implementation. If needed, uncomment the below lines
		//caConfig, mappedHost := m.tryMatchingCAConfig(networkConfig, strings.ToLower(certAuthorityName))
		//if mappedHost == "" {
		return nil, errors.Errorf("CA Server Name %s not found", certAuthorityName)
		//}
		//return caConfig, nil
	}

	return &caConfig, nil
}

type exampleCaServerCerts struct{}

func (m *exampleCaServerCerts) CAServerCerts(org string) ([][]byte, error) {
	caConfig, err := getCAConfig(&networkConfig, org)
	if err != nil {
		return nil, err
	}

	var serverCerts [][]byte
	//check for pems first
	pems := caConfig.TLSCACerts.Pem
	if len(pems) > 0 {
		serverCerts = make([][]byte, len(pems))
		for i, pem := range pems {
			serverCerts[i] = []byte(pem)
		}
		return serverCerts, nil
	}

	//check for files if pems not found
	certFiles := strings.Split(caConfig.TLSCACerts.Path, ",")
	serverCerts = make([][]byte, len(certFiles))
	for i, certPath := range certFiles {
		bytes, err := ioutil.ReadFile(pathvar.Subst(certPath))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to load pem bytes from path %s", certPath)
		}
		serverCerts[i] = bytes
	}
	return serverCerts, nil
}

type exampleCaClientKey struct{}

func (m *exampleCaClientKey) CAClientKey(org string) ([]byte, error) {
	caConfig, err := getCAConfig(&networkConfig, org)
	if err != nil {
		return nil, err
	}

	//subst path
	caConfig.TLSCACerts.Client.Key.Path = pathvar.Subst(caConfig.TLSCACerts.Client.Key.Path)

	return caConfig.TLSCACerts.Client.Key.Bytes()
}

type exampleCaClientCert struct{}

func (m *exampleCaClientCert) CAClientCert(org string) ([]byte, error) {
	caConfig, err := getCAConfig(&networkConfig, org)
	if err != nil {
		return nil, err
	}

	//subst path
	caConfig.TLSCACerts.Client.Cert.Path = pathvar.Subst(caConfig.TLSCACerts.Client.Cert.Path)

	return caConfig.TLSCACerts.Client.Cert.Bytes()
}

type exampleCaKeyStorePath struct{}

func (m *exampleCaKeyStorePath) CAKeyStorePath() string {
	return "/tmp/msp"
}

type exampleCredentialStorePath struct{}

func (m *exampleCredentialStorePath) CredentialStorePath() string {
	return "/tmp/state-store"
}

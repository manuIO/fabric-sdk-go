/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/hyperledger/fabric-sdk-go/pkg/core/config/cryptoutil"

	fabricCaUtil "github.com/hyperledger/fabric-sdk-go/internal/github.com/hyperledger/fabric-ca/util"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/core"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/msp"
	"github.com/hyperledger/fabric-sdk-go/pkg/util/pathvar"
	"github.com/pkg/errors"
)

func newUser(userData *msp.UserData, cryptoSuite core.CryptoSuite) (*User, error) {
	pubKey, err := cryptoutil.GetPublicKeyFromCert(userData.EnrollmentCertificate, cryptoSuite)
	if err != nil {
		return nil, errors.WithMessage(err, "fetching public key from cert failed")
	}
	pk, err := cryptoSuite.GetKey(pubKey.SKI())
	if err != nil {
		return nil, errors.WithMessage(err, "cryptoSuite GetKey failed")
	}
	u := &User{
		id:    userData.ID,
		mspID: userData.MSPID,
		enrollmentCertificate: userData.EnrollmentCertificate,
		privateKey:            pk,
	}
	return u, nil
}

// NewUser creates a User instance
func (mgr *IdentityManager) NewUser(userData *msp.UserData) (*User, error) {
	return newUser(userData, mgr.cryptoSuite)
}

func (mgr *IdentityManager) loadUserFromStore(username string) (*User, error) {
	if mgr.userStore == nil {
		return nil, msp.ErrUserNotFound
	}
	var user *User
	userData, err := mgr.userStore.Load(msp.IdentityIdentifier{MSPID: mgr.orgMSPID, ID: username})
	if err != nil {
		return nil, err
	}
	user, err = mgr.NewUser(userData)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetSigningIdentity returns a signing identity for the given id
func (mgr *IdentityManager) GetSigningIdentity(id string) (msp.SigningIdentity, error) {
	user, err := mgr.GetUser(id)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetUser returns a user for the given user name
func (mgr *IdentityManager) GetUser(username string) (*User, error) { //nolint

	u, err := mgr.loadUserFromStore(username)
	if err != nil {
		if err != msp.ErrUserNotFound {
			return nil, errors.WithMessage(err, "loading user from store failed")
		}
		// Not found, continue
	}

	if u == nil {
		certBytes, err := mgr.getEmbeddedCertBytes(username)
		if err != nil && err != msp.ErrUserNotFound {
			return nil, errors.WithMessage(err, "fetching embedded cert failed")
		}
		if certBytes == nil {
			certBytes, err = mgr.getCertBytesFromCertStore(username)
			if err != nil && err != msp.ErrUserNotFound {
				return nil, errors.WithMessage(err, "fetching cert from store failed")
			}
		}
		if certBytes == nil {
			return nil, msp.ErrUserNotFound
		}
		privateKey, err := mgr.getEmbeddedPrivateKey(username)
		if err != nil {
			return nil, errors.WithMessage(err, "fetching embedded private key failed")
		}
		if privateKey == nil {
			privateKey, err = mgr.getPrivateKeyFromCert(username, certBytes)
			if err != nil {
				return nil, errors.WithMessage(err, "getting private key from cert failed")
			}
		}
		if privateKey == nil {
			return nil, fmt.Errorf("unable to find private key for user [%s]", username)
		}
		mspID, ok := mgr.config.MSPID(mgr.orgName)
		if !ok {
			return nil, errors.New("MSP ID config read failed")
		}
		u = &User{
			id:    username,
			mspID: mspID,
			enrollmentCertificate: certBytes,
			privateKey:            privateKey,
		}
	}
	return u, nil
}

func (mgr *IdentityManager) getEmbeddedCertBytes(username string) ([]byte, error) {
	certPem := mgr.embeddedUsers[strings.ToLower(username)].Cert.Pem
	certPath := pathvar.Subst(mgr.embeddedUsers[strings.ToLower(username)].Cert.Path)

	if certPem == "" && certPath == "" {
		return nil, msp.ErrUserNotFound
	}

	var pemBytes []byte
	var err error

	if certPem != "" {
		pemBytes = []byte(certPem)
	} else if certPath != "" {
		pemBytes, err = ioutil.ReadFile(certPath)
		if err != nil {
			return nil, errors.WithMessage(err, "reading cert from embedded path failed")
		}
	}

	return pemBytes, nil
}

func (mgr *IdentityManager) getEmbeddedPrivateKey(username string) (core.Key, error) {
	keyPem := mgr.embeddedUsers[strings.ToLower(username)].Key.Pem
	keyPath := pathvar.Subst(mgr.embeddedUsers[strings.ToLower(username)].Key.Path)

	var privateKey core.Key
	var pemBytes []byte
	var err error

	if keyPem != "" {
		// Try importing from the Embedded Pem
		pemBytes = []byte(keyPem)
	} else if keyPath != "" {
		// Try importing from the Embedded Path
		_, err = os.Stat(keyPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, errors.WithMessage(err, "OS stat embedded path failed")
			}
			// file doesn't exist, continue
		} else {
			// file exists, try to read it
			pemBytes, err = ioutil.ReadFile(keyPath)
			if err != nil {
				return nil, errors.WithMessage(err, "reading private key from embedded path failed")
			}
		}
	}

	if pemBytes != nil {
		// Try the crypto provider as a SKI
		privateKey, err = mgr.cryptoSuite.GetKey(pemBytes)
		if err != nil || privateKey == nil {
			// Try as a pem
			privateKey, err = fabricCaUtil.ImportBCCSPKeyFromPEMBytes(pemBytes, mgr.cryptoSuite, true)
			if err != nil {
				return nil, errors.Wrap(err, "import private key failed")
			}
		}
	}

	return privateKey, nil
}

func (mgr *IdentityManager) getPrivateKeyPemFromKeyStore(username string, ski []byte) ([]byte, error) {
	if mgr.mspPrivKeyStore == nil {
		return nil, nil
	}
	key, err := mgr.mspPrivKeyStore.Load(
		&msp.PrivKeyKey{
			ID:    username,
			MSPID: mgr.orgMSPID,
			SKI:   ski,
		})
	if err != nil {
		return nil, err
	}
	keyBytes, ok := key.([]byte)
	if !ok {
		return nil, errors.New("key from store is not []byte")
	}
	return keyBytes, nil
}

func (mgr *IdentityManager) getCertBytesFromCertStore(username string) ([]byte, error) {
	if mgr.mspCertStore == nil {
		return nil, msp.ErrUserNotFound
	}
	cert, err := mgr.mspCertStore.Load(&msp.IdentityIdentifier{
		ID:    username,
		MSPID: mgr.orgMSPID,
	})
	if err != nil {
		if err == core.ErrKeyValueNotFound {
			return nil, msp.ErrUserNotFound
		}
		return nil, err
	}
	certBytes, ok := cert.([]byte)
	if !ok {
		return nil, errors.New("cert from store is not []byte")
	}
	return certBytes, nil
}

func (mgr *IdentityManager) getPrivateKeyFromCert(username string, cert []byte) (core.Key, error) {
	if cert == nil {
		return nil, errors.New("cert is nil")
	}
	pubKey, err := cryptoutil.GetPublicKeyFromCert(cert, mgr.cryptoSuite)
	if err != nil {
		return nil, errors.WithMessage(err, "fetching public key from cert failed")
	}
	privKey, err := mgr.getPrivateKeyFromKeyStore(username, pubKey.SKI())
	if err == nil {
		return privKey, nil
	}
	if err != core.ErrKeyValueNotFound {
		return nil, errors.WithMessage(err, "fetching private key from key store failed")
	}
	return mgr.cryptoSuite.GetKey(pubKey.SKI())
}

func (mgr *IdentityManager) getPrivateKeyFromKeyStore(username string, ski []byte) (core.Key, error) {
	pemBytes, err := mgr.getPrivateKeyPemFromKeyStore(username, ski)
	if err != nil {
		return nil, err
	}
	if pemBytes != nil {
		return fabricCaUtil.ImportBCCSPKeyFromPEMBytes(pemBytes, mgr.cryptoSuite, true)
	}
	return nil, core.ErrKeyValueNotFound
}

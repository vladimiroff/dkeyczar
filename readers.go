package dkeyczar

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"os"
	"strconv"
)

// KeyReader provides an interface for returning information about a particular key.
type KeyReader interface {
	// GetMetadata returns the meta information for this key
	GetMetadata() (string, error)
	// GetKey returns the key material for a particular version of this key
	GetKey(version int) (string, error)
}

type fileReader struct {
	location string // directory path of keyfiles
}

// NewFileReader returns a KeyReader that reads a keyczar key from a directory on the file system.
func NewFileReader(location string) KeyReader {
	r := new(fileReader)

	// make sure 'location' ends with our path separator
	if location[len(location)-1] == os.PathSeparator {
		r.location = location
	} else {
		r.location = location + string(os.PathSeparator)
	}

	return r
}

type encryptedReader struct {
	reader  KeyReader
	crypter Crypter
}

// NewEncryptedReader returns a KeyReader which decrypts the key returned by the wrapped 'reader'.
func NewEncryptedReader(reader KeyReader, crypter Crypter) KeyReader {
	r := new(encryptedReader)

	r.crypter = crypter
	r.reader = reader

	return r
}

// return the entire contents of a file as a string
func slurp(path string) (string, error) {
	f, err := os.Open(path)

	if err != nil {
		return "", err
	}

	meta := make([]byte, 0, 512)
	var buf [512]byte

	for {
		n, err := f.Read(buf[:])
		if n == 0 && err == io.EOF {
			break
		}

		if err != nil {
			return "", err
		}

		meta = append(meta, buf[0:n]...)
	}

	return string(meta), nil
}

// slurp and return the meta file
func (r *fileReader) GetMetadata() (string, error) {
	return slurp(r.location + "meta")
}

// slurp and return the requested key version
func (r *fileReader) GetKey(version int) (string, error) {
	return slurp(r.location + strconv.Itoa(version))
}

// return the meta information from the wrapper reader.  Meta information is not encrypted.
func (r *encryptedReader) GetMetadata() (string, error) {
	return r.reader.GetMetadata()
}

// decrypt and return an encrypted key
func (r *encryptedReader) GetKey(version int) (string, error) {
	s, err := r.reader.GetKey(version)

	if err != nil {
		return "", err

	}

	b, err := r.crypter.Decrypt(s)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

type importedRsaPrivateKeyReader struct {
	km      keyMeta
	rsajson rsaKeyJSON
}

// ugly name -- if we exported key purpose it would be easier
func newImportedRsaPrivateKeyReader(key *rsa.PrivateKey) KeyReader {
	r := new(importedRsaPrivateKeyReader)
	kv := keyVersion{1, ksPRIMARY, false}
	r.km = keyMeta{"Imported RSA Private Key", ktRSA_PRIV, kpDECRYPT_AND_ENCRYPT, false, []keyVersion{kv}}

	// inverse of code with newRsaKeys
	r.rsajson.PublicKey.Modulus = encodeWeb64String(key.PublicKey.N.Bytes())

	e := big.NewInt(int64(key.PublicKey.E))
	r.rsajson.PublicKey.PublicExponent = encodeWeb64String(e.Bytes())

	r.rsajson.PrimeP = encodeWeb64String(key.Primes[0].Bytes())
	r.rsajson.PrimeQ = encodeWeb64String(key.Primes[1].Bytes())
	r.rsajson.PrivateExponent = encodeWeb64String(key.D.Bytes())
	r.rsajson.PrimeExponentP = encodeWeb64String(key.Precomputed.Dp.Bytes())
	r.rsajson.PrimeExponentQ = encodeWeb64String(key.Precomputed.Dq.Bytes())
	r.rsajson.CrtCoefficient = encodeWeb64String(key.Precomputed.Qinv.Bytes())

	return r
}

func (r *importedRsaPrivateKeyReader) GetMetadata() (string, error) {
	b, err := json.Marshal(r.km)
	return string(b), err
}

func (r *importedRsaPrivateKeyReader) GetKey(version int) (string, error) {
	b, err := json.Marshal(r.rsajson)
	return string(b), err
}

// ImportRSAKeyFromPEM returns a KeyReader for the RSA Private Key contained in the PEM file specified in the location.
func ImportRSAKeyFromPEM(location string) (KeyReader, error) {

	buf, _ := slurp(location)

	block, _ := pem.Decode([]byte(buf))
	priv, _ := x509.ParsePKCS1PrivateKey(block.Bytes)

	r := newImportedRsaPrivateKeyReader(priv)

	return r, nil

}

type importedRsaPublicKeyReader struct {
	km      keyMeta
	rsajson rsaPublicKeyJSON
}

func newImportedRsaPublicKeyReader(key *rsa.PublicKey) KeyReader {
	r := new(importedRsaPublicKeyReader)
	kv := keyVersion{1, ksPRIMARY, false}
	r.km = keyMeta{"Imported RSA Public Key", ktRSA_PUB, kpENCRYPT, false, []keyVersion{kv}}

	// inverse of code with newRsaKeys
	r.rsajson.Modulus = encodeWeb64String(key.N.Bytes())

	e := big.NewInt(int64(key.E))
	r.rsajson.PublicExponent = encodeWeb64String(e.Bytes())

	return r
}

func (r *importedRsaPublicKeyReader) GetMetadata() (string, error) {
	b, err := json.Marshal(r.km)
	return string(b), err
}

func (r *importedRsaPublicKeyReader) GetKey(version int) (string, error) {
	b, err := json.Marshal(r.rsajson)
	return string(b), err
}

// ImportRSAPublicKeyFromPEM returns a KeyReader for the RSA Public Key contained in the PEM file specified in the location.
func ImportRSAPublicKeyFromPEM(location string) (KeyReader, error) {

	buf, _ := slurp(location)

	block, _ := pem.Decode([]byte(buf))
	pub, _ := x509.ParsePKIXPublicKey(block.Bytes)

	rsapub := pub.(*rsa.PublicKey)

	r := newImportedRsaPublicKeyReader(rsapub)

	return r, nil

}

type importedAesKeyReader struct {
	km      keyMeta
	aesjson aesKeyJSON
}

func newImportedAesKeyReader(key *aesKey) KeyReader {
	r := new(importedAesKeyReader)
	kv := keyVersion{1, ksPRIMARY, false}
	r.km = keyMeta{"Imported AES Key", ktAES, kpDECRYPT_AND_ENCRYPT, false, []keyVersion{kv}}

	// inverse of code with newAesKeys
	r.aesjson.AesKeyString = encodeWeb64String(key.key)
	r.aesjson.Size = len(key.key) * 8
	r.aesjson.HmacKey.HmacKeyString = encodeWeb64String(key.hmacKey.key)
	r.aesjson.HmacKey.Size = len(key.hmacKey.key) * 8
	r.aesjson.Mode = cmCBC

	return r
}

func (r *importedAesKeyReader) GetMetadata() (string, error) {
	b, err := json.Marshal(r.km)
	return string(b), err
}

func (r *importedAesKeyReader) GetKey(version int) (string, error) {
	b, err := json.Marshal(r.aesjson)
	return string(b), err
}

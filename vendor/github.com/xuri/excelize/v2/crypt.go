// Copyright 2016 - 2022 The excelize Authors. All rights reserved. Use of
// this source code is governed by a BSD-style license that can be found in
// the LICENSE file.
//
// Package excelize providing a set of functions that allow you to write to and
// read from XLAM / XLSM / XLSX / XLTM / XLTX files. Supports reading and
// writing spreadsheet documents generated by Microsoft Excel™ 2007 and later.
// Supports complex components by high compatibility, and provided streaming
// API for generating or reading data from a worksheet with huge amounts of
// data. This library needs Go version 1.15 or later.

package excelize

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"hash"
	"math"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/richardlehane/mscfb"
	"golang.org/x/crypto/md4"
	"golang.org/x/crypto/ripemd160"
	"golang.org/x/text/encoding/unicode"
)

var (
	blockKey                   = []byte{0x14, 0x6e, 0x0b, 0xe7, 0xab, 0xac, 0xd0, 0xd6} // Block keys used for encryption
	oleIdentifier              = []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}
	headerCLSID                = make([]byte, 16)
	difSect                    = -4
	endOfChain                 = -2
	fatSect                    = -3
	iterCount                  = 50000
	packageEncryptionChunkSize = 4096
	packageOffset              = 8 // First 8 bytes are the size of the stream
	sheetProtectionSpinCount   = 1e5
)

// Encryption specifies the encryption structure, streams, and storages are
// required when encrypting ECMA-376 documents.
type Encryption struct {
	XMLName       xml.Name      `xml:"encryption"`
	KeyData       KeyData       `xml:"keyData"`
	DataIntegrity DataIntegrity `xml:"dataIntegrity"`
	KeyEncryptors KeyEncryptors `xml:"keyEncryptors"`
}

// KeyData specifies the cryptographic attributes used to encrypt the data.
type KeyData struct {
	SaltSize        int    `xml:"saltSize,attr"`
	BlockSize       int    `xml:"blockSize,attr"`
	KeyBits         int    `xml:"keyBits,attr"`
	HashSize        int    `xml:"hashSize,attr"`
	CipherAlgorithm string `xml:"cipherAlgorithm,attr"`
	CipherChaining  string `xml:"cipherChaining,attr"`
	HashAlgorithm   string `xml:"hashAlgorithm,attr"`
	SaltValue       string `xml:"saltValue,attr"`
}

// DataIntegrity specifies the encrypted copies of the salt and hash values
// used to help ensure that the integrity of the encrypted data has not been
// compromised.
type DataIntegrity struct {
	EncryptedHmacKey   string `xml:"encryptedHmacKey,attr"`
	EncryptedHmacValue string `xml:"encryptedHmacValue,attr"`
}

// KeyEncryptors specifies the key encryptors used to encrypt the data.
type KeyEncryptors struct {
	KeyEncryptor []KeyEncryptor `xml:"keyEncryptor"`
}

// KeyEncryptor specifies that the schema used by this encryptor is the schema
// specified for password-based encryptors.
type KeyEncryptor struct {
	XMLName      xml.Name     `xml:"keyEncryptor"`
	URI          string       `xml:"uri,attr"`
	EncryptedKey EncryptedKey `xml:"encryptedKey"`
}

// EncryptedKey used to generate the encrypting key.
type EncryptedKey struct {
	XMLName                    xml.Name `xml:"http://schemas.microsoft.com/office/2006/keyEncryptor/password encryptedKey"`
	SpinCount                  int      `xml:"spinCount,attr"`
	EncryptedVerifierHashInput string   `xml:"encryptedVerifierHashInput,attr"`
	EncryptedVerifierHashValue string   `xml:"encryptedVerifierHashValue,attr"`
	EncryptedKeyValue          string   `xml:"encryptedKeyValue,attr"`
	KeyData
}

// StandardEncryptionHeader structure is used by ECMA-376 document encryption
// [ECMA-376] and Office binary document RC4 CryptoAPI encryption, to specify
// encryption properties for an encrypted stream.
type StandardEncryptionHeader struct {
	Flags        uint32
	SizeExtra    uint32
	AlgID        uint32
	AlgIDHash    uint32
	KeySize      uint32
	ProviderType uint32
	Reserved1    uint32
	Reserved2    uint32
	CspName      string
}

// StandardEncryptionVerifier structure is used by Office Binary Document RC4
// CryptoAPI Encryption and ECMA-376 Document Encryption. Every usage of this
// structure MUST specify the hashing algorithm and encryption algorithm used
// in the EncryptionVerifier structure.
type StandardEncryptionVerifier struct {
	SaltSize              uint32
	Salt                  []byte
	EncryptedVerifier     []byte
	VerifierHashSize      uint32
	EncryptedVerifierHash []byte
}

// encryptionInfo structure is used for standard encryption with SHA1
// cryptographic algorithm.
type encryption struct {
	BlockSize, SaltSize                                                                  int
	EncryptedKeyValue, EncryptedVerifierHashInput, EncryptedVerifierHashValue, SaltValue []byte
	KeyBits                                                                              uint32
}

// Decrypt API decrypts the CFB file format with ECMA-376 agile encryption and
// standard encryption. Support cryptographic algorithm: MD4, MD5, RIPEMD-160,
// SHA1, SHA256, SHA384 and SHA512 currently.
func Decrypt(raw []byte, opt *Options) (packageBuf []byte, err error) {
	doc, err := mscfb.New(bytes.NewReader(raw))
	if err != nil {
		return
	}
	encryptionInfoBuf, encryptedPackageBuf := extractPart(doc)
	mechanism, err := encryptionMechanism(encryptionInfoBuf)
	if err != nil || mechanism == "extensible" {
		return
	}
	if mechanism == "agile" {
		return agileDecrypt(encryptionInfoBuf, encryptedPackageBuf, opt)
	}
	return standardDecrypt(encryptionInfoBuf, encryptedPackageBuf, opt)
}

// Encrypt API encrypt data with the password.
func Encrypt(raw []byte, opt *Options) ([]byte, error) {
	encryptor := encryption{
		EncryptedVerifierHashInput: make([]byte, 16),
		EncryptedVerifierHashValue: make([]byte, 32),
		SaltValue:                  make([]byte, 16),
		BlockSize:                  16,
		KeyBits:                    128,
		SaltSize:                   16,
	}
	// Key Encryption
	encryptionInfoBuffer, err := encryptor.standardKeyEncryption(opt.Password)
	if err != nil {
		return nil, err
	}
	// Package Encryption
	encryptedPackage := make([]byte, 8)
	binary.LittleEndian.PutUint64(encryptedPackage, uint64(len(raw)))
	encryptedPackage = append(encryptedPackage, encryptor.encrypt(raw)...)
	// Create a new CFB
	compoundFile := &cfb{
		paths:   []string{"Root Entry/"},
		sectors: []sector{{name: "Root Entry", typeID: 5}},
	}
	compoundFile.put("EncryptionInfo", encryptionInfoBuffer)
	compoundFile.put("EncryptedPackage", encryptedPackage)
	return compoundFile.write(), nil
}

// extractPart extract data from storage by specified part name.
func extractPart(doc *mscfb.Reader) (encryptionInfoBuf, encryptedPackageBuf []byte) {
	for entry, err := doc.Next(); err == nil; entry, err = doc.Next() {
		switch entry.Name {
		case "EncryptionInfo":
			buf := make([]byte, entry.Size)
			i, _ := doc.Read(buf)
			if i > 0 {
				encryptionInfoBuf = buf
			}
		case "EncryptedPackage":
			buf := make([]byte, entry.Size)
			i, _ := doc.Read(buf)
			if i > 0 {
				encryptedPackageBuf = buf
			}
		}
	}
	return
}

// encryptionMechanism parse password-protected documents created mechanism.
func encryptionMechanism(buffer []byte) (mechanism string, err error) {
	if len(buffer) < 4 {
		err = ErrUnknownEncryptMechanism
		return
	}
	versionMajor, versionMinor := binary.LittleEndian.Uint16(buffer[:2]), binary.LittleEndian.Uint16(buffer[2:4])
	if versionMajor == 4 && versionMinor == 4 {
		mechanism = "agile"
		return
	} else if (2 <= versionMajor && versionMajor <= 4) && versionMinor == 2 {
		mechanism = "standard"
		return
	} else if (versionMajor == 3 || versionMajor == 4) && versionMinor == 3 {
		mechanism = "extensible"
	}
	err = ErrUnsupportedEncryptMechanism
	return
}

// ECMA-376 Standard Encryption

// standardDecrypt decrypt the CFB file format with ECMA-376 standard encryption.
func standardDecrypt(encryptionInfoBuf, encryptedPackageBuf []byte, opt *Options) ([]byte, error) {
	encryptionHeaderSize := binary.LittleEndian.Uint32(encryptionInfoBuf[8:12])
	block := encryptionInfoBuf[12 : 12+encryptionHeaderSize]
	header := StandardEncryptionHeader{
		Flags:        binary.LittleEndian.Uint32(block[:4]),
		SizeExtra:    binary.LittleEndian.Uint32(block[4:8]),
		AlgID:        binary.LittleEndian.Uint32(block[8:12]),
		AlgIDHash:    binary.LittleEndian.Uint32(block[12:16]),
		KeySize:      binary.LittleEndian.Uint32(block[16:20]),
		ProviderType: binary.LittleEndian.Uint32(block[20:24]),
		Reserved1:    binary.LittleEndian.Uint32(block[24:28]),
		Reserved2:    binary.LittleEndian.Uint32(block[28:32]),
		CspName:      string(block[32:]),
	}
	block = encryptionInfoBuf[12+encryptionHeaderSize:]
	algIDMap := map[uint32]string{
		0x0000660E: "AES-128",
		0x0000660F: "AES-192",
		0x00006610: "AES-256",
	}
	algorithm := "AES"
	_, ok := algIDMap[header.AlgID]
	if !ok {
		algorithm = "RC4"
	}
	verifier := standardEncryptionVerifier(algorithm, block)
	secretKey, err := standardConvertPasswdToKey(header, verifier, opt)
	if err != nil {
		return nil, err
	}
	// decrypted data
	x := encryptedPackageBuf[8:]
	blob, err := aes.NewCipher(secretKey)
	if err != nil {
		return nil, err
	}
	decrypted := make([]byte, len(x))
	size := 16
	for bs, be := 0, size; bs < len(x); bs, be = bs+size, be+size {
		blob.Decrypt(decrypted[bs:be], x[bs:be])
	}
	return decrypted, err
}

// standardEncryptionVerifier extract ECMA-376 standard encryption verifier.
func standardEncryptionVerifier(algorithm string, blob []byte) StandardEncryptionVerifier {
	verifier := StandardEncryptionVerifier{
		SaltSize:          binary.LittleEndian.Uint32(blob[:4]),
		Salt:              blob[4:20],
		EncryptedVerifier: blob[20:36],
		VerifierHashSize:  binary.LittleEndian.Uint32(blob[36:40]),
	}
	if algorithm == "RC4" {
		verifier.EncryptedVerifierHash = blob[40:60]
	} else if algorithm == "AES" {
		verifier.EncryptedVerifierHash = blob[40:72]
	}
	return verifier
}

// standardConvertPasswdToKey generate intermediate key from given password.
func standardConvertPasswdToKey(header StandardEncryptionHeader, verifier StandardEncryptionVerifier, opt *Options) ([]byte, error) {
	encoder := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewEncoder()
	passwordBuffer, err := encoder.Bytes([]byte(opt.Password))
	if err != nil {
		return nil, err
	}
	key := hashing("sha1", verifier.Salt, passwordBuffer)
	for i := 0; i < iterCount; i++ {
		iterator := createUInt32LEBuffer(i, 4)
		key = hashing("sha1", iterator, key)
	}
	var block int
	hFinal := hashing("sha1", key, createUInt32LEBuffer(block, 4))
	cbRequiredKeyLength := int(header.KeySize) / 8
	cbHash := sha1.Size
	buf1 := bytes.Repeat([]byte{0x36}, 64)
	buf1 = append(standardXORBytes(hFinal, buf1[:cbHash]), buf1[cbHash:]...)
	x1 := hashing("sha1", buf1)
	buf2 := bytes.Repeat([]byte{0x5c}, 64)
	buf2 = append(standardXORBytes(hFinal, buf2[:cbHash]), buf2[cbHash:]...)
	x2 := hashing("sha1", buf2)
	x3 := append(x1, x2...)
	keyDerived := x3[:cbRequiredKeyLength]
	return keyDerived, err
}

// standardXORBytes perform XOR operations for two bytes slice.
func standardXORBytes(a, b []byte) []byte {
	r := make([][2]byte, len(a))
	for i, e := range a {
		r[i] = [2]byte{e, b[i]}
	}
	buf := make([]byte, len(a))
	for p, q := range r {
		buf[p] = q[0] ^ q[1]
	}
	return buf
}

// encrypt provides a function to encrypt given value with AES cryptographic
// algorithm.
func (e *encryption) encrypt(input []byte) []byte {
	inputBytes := len(input)
	if pad := inputBytes % e.BlockSize; pad != 0 {
		inputBytes += e.BlockSize - pad
	}
	var output, chunk []byte
	encryptedChunk := make([]byte, e.BlockSize)
	for i := 0; i < inputBytes; i += e.BlockSize {
		if i+e.BlockSize <= len(input) {
			chunk = input[i : i+e.BlockSize]
		} else {
			chunk = input[i:]
		}
		chunk = append(chunk, make([]byte, e.BlockSize-len(chunk))...)
		c, _ := aes.NewCipher(e.EncryptedKeyValue)
		c.Encrypt(encryptedChunk, chunk)
		output = append(output, encryptedChunk...)
	}
	return output
}

// standardKeyEncryption encrypt convert the password to an encryption key.
func (e *encryption) standardKeyEncryption(password string) ([]byte, error) {
	if len(password) == 0 || len(password) > MaxFieldLength {
		return nil, ErrPasswordLengthInvalid
	}
	var storage cfb
	storage.writeUint16(0x0003)
	storage.writeUint16(0x0002)
	storage.writeUint32(0x24)
	storage.writeUint32(0xA4)
	storage.writeUint32(0x24)
	storage.writeUint32(0x00)
	storage.writeUint32(0x660E)
	storage.writeUint32(0x8004)
	storage.writeUint32(0x80)
	storage.writeUint32(0x18)
	storage.writeUint64(0x00)
	providerName := "Microsoft Enhanced RSA and AES Cryptographic Provider (Prototype)"
	storage.writeStrings(providerName)
	storage.writeUint16(0x00)
	storage.writeUint32(0x10)
	keyDataSaltValue, _ := randomBytes(16)
	verifierHashInput, _ := randomBytes(16)
	e.SaltValue = keyDataSaltValue
	e.EncryptedKeyValue, _ = standardConvertPasswdToKey(
		StandardEncryptionHeader{KeySize: e.KeyBits},
		StandardEncryptionVerifier{Salt: e.SaltValue},
		&Options{Password: password})
	verifierHashInputKey := hashing("sha1", verifierHashInput)
	e.EncryptedVerifierHashInput = e.encrypt(verifierHashInput)
	e.EncryptedVerifierHashValue = e.encrypt(verifierHashInputKey)
	storage.writeBytes(e.SaltValue)
	storage.writeBytes(e.EncryptedVerifierHashInput)
	storage.writeUint32(0x14)
	storage.writeBytes(e.EncryptedVerifierHashValue)
	storage.position = 0
	return storage.stream, nil
}

// ECMA-376 Agile Encryption

// agileDecrypt decrypt the CFB file format with ECMA-376 agile encryption.
// Support cryptographic algorithm: MD4, MD5, RIPEMD-160, SHA1, SHA256,
// SHA384 and SHA512.
func agileDecrypt(encryptionInfoBuf, encryptedPackageBuf []byte, opt *Options) (packageBuf []byte, err error) {
	var encryptionInfo Encryption
	if encryptionInfo, err = parseEncryptionInfo(encryptionInfoBuf[8:]); err != nil {
		return
	}
	// Convert the password into an encryption key.
	key, err := convertPasswdToKey(opt.Password, blockKey, encryptionInfo)
	if err != nil {
		return
	}
	// Use the key to decrypt the package key.
	encryptedKey := encryptionInfo.KeyEncryptors.KeyEncryptor[0].EncryptedKey
	saltValue, err := base64.StdEncoding.DecodeString(encryptedKey.SaltValue)
	if err != nil {
		return
	}
	encryptedKeyValue, err := base64.StdEncoding.DecodeString(encryptedKey.EncryptedKeyValue)
	if err != nil {
		return
	}
	packageKey, _ := decrypt(key, saltValue, encryptedKeyValue)
	// Use the package key to decrypt the package.
	return decryptPackage(packageKey, encryptedPackageBuf, encryptionInfo)
}

// convertPasswdToKey convert the password into an encryption key.
func convertPasswdToKey(passwd string, blockKey []byte, encryption Encryption) (key []byte, err error) {
	var b bytes.Buffer
	saltValue, err := base64.StdEncoding.DecodeString(encryption.KeyEncryptors.KeyEncryptor[0].EncryptedKey.SaltValue)
	if err != nil {
		return
	}
	b.Write(saltValue)
	encoder := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewEncoder()
	passwordBuffer, err := encoder.Bytes([]byte(passwd))
	if err != nil {
		return
	}
	b.Write(passwordBuffer)
	// Generate the initial hash.
	key = hashing(encryption.KeyData.HashAlgorithm, b.Bytes())
	// Now regenerate until spin count.
	for i := 0; i < encryption.KeyEncryptors.KeyEncryptor[0].EncryptedKey.SpinCount; i++ {
		iterator := createUInt32LEBuffer(i, 4)
		key = hashing(encryption.KeyData.HashAlgorithm, iterator, key)
	}
	// Now generate the final hash.
	key = hashing(encryption.KeyData.HashAlgorithm, key, blockKey)
	// Truncate or pad as needed to get to length of keyBits.
	keyBytes := encryption.KeyEncryptors.KeyEncryptor[0].EncryptedKey.KeyBits / 8
	if len(key) < keyBytes {
		tmp := make([]byte, 0x36)
		key = append(key, tmp...)
	} else if len(key) > keyBytes {
		key = key[:keyBytes]
	}
	return
}

// hashing data by specified hash algorithm.
func hashing(hashAlgorithm string, buffer ...[]byte) (key []byte) {
	hashMap := map[string]hash.Hash{
		"md4":        md4.New(),
		"md5":        md5.New(),
		"ripemd-160": ripemd160.New(),
		"sha1":       sha1.New(),
		"sha256":     sha256.New(),
		"sha384":     sha512.New384(),
		"sha512":     sha512.New(),
	}
	handler, ok := hashMap[strings.ToLower(hashAlgorithm)]
	if !ok {
		return key
	}
	for _, buf := range buffer {
		_, _ = handler.Write(buf)
	}
	key = handler.Sum(nil)
	return key
}

// createUInt32LEBuffer create buffer with little endian 32-bit unsigned
// integer.
func createUInt32LEBuffer(value int, bufferSize int) []byte {
	buf := make([]byte, bufferSize)
	binary.LittleEndian.PutUint32(buf, uint32(value))
	return buf
}

// parseEncryptionInfo parse the encryption info XML into an object.
func parseEncryptionInfo(encryptionInfo []byte) (encryption Encryption, err error) {
	err = xml.Unmarshal(encryptionInfo, &encryption)
	return
}

// decrypt provides a function to decrypt input by given cipher algorithm,
// cipher chaining, key and initialization vector.
func decrypt(key, iv, input []byte) (packageKey []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return input, err
	}
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(input, input)
	return input, nil
}

// decryptPackage decrypt package by given packageKey and encryption
// info.
func decryptPackage(packageKey, input []byte, encryption Encryption) (outputChunks []byte, err error) {
	encryptedKey, offset := encryption.KeyData, packageOffset
	var i, start, end int
	var iv, outputChunk []byte
	for end < len(input) {
		start = end
		end = start + packageEncryptionChunkSize

		if end > len(input) {
			end = len(input)
		}
		// Grab the next chunk
		var inputChunk []byte
		if (end + offset) < len(input) {
			inputChunk = input[start+offset : end+offset]
		} else {
			inputChunk = input[start+offset : end]
		}

		// Pad the chunk if it is not an integer multiple of the block size
		remainder := len(inputChunk) % encryptedKey.BlockSize
		if remainder != 0 {
			inputChunk = append(inputChunk, make([]byte, encryptedKey.BlockSize-remainder)...)
		}
		// Create the initialization vector
		iv, err = createIV(i, encryption)
		if err != nil {
			return
		}
		// Decrypt the chunk and add it to the array
		outputChunk, err = decrypt(packageKey, iv, inputChunk)
		if err != nil {
			return
		}
		outputChunks = append(outputChunks, outputChunk...)
		i++
	}
	return
}

// createIV create an initialization vector (IV).
func createIV(blockKey interface{}, encryption Encryption) ([]byte, error) {
	encryptedKey := encryption.KeyData
	// Create the block key from the current index
	var blockKeyBuf []byte
	if reflect.TypeOf(blockKey).Kind() == reflect.Int {
		blockKeyBuf = createUInt32LEBuffer(blockKey.(int), 4)
	} else {
		blockKeyBuf = blockKey.([]byte)
	}
	saltValue, err := base64.StdEncoding.DecodeString(encryptedKey.SaltValue)
	if err != nil {
		return nil, err
	}
	// Create the initialization vector by hashing the salt with the block key.
	// Truncate or pad as needed to meet the block size.
	iv := hashing(encryptedKey.HashAlgorithm, append(saltValue, blockKeyBuf...))
	if len(iv) < encryptedKey.BlockSize {
		tmp := make([]byte, 0x36)
		iv = append(iv, tmp...)
	} else if len(iv) > encryptedKey.BlockSize {
		iv = iv[:encryptedKey.BlockSize]
	}
	return iv, nil
}

// randomBytes returns securely generated random bytes. It will return an
// error if the system's secure random number generator fails to function
// correctly, in which case the caller should not continue.
func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}

// ISO Write Protection Method

// genISOPasswdHash implements the ISO password hashing algorithm by given
// plaintext password, name of the cryptographic hash algorithm, salt value
// and spin count.
func genISOPasswdHash(passwd, hashAlgorithm, salt string, spinCount int) (hashValue, saltValue string, err error) {
	if len(passwd) < 1 || len(passwd) > MaxFieldLength {
		err = ErrPasswordLengthInvalid
		return
	}
	algorithmName, ok := map[string]string{
		"MD4":     "md4",
		"MD5":     "md5",
		"SHA-1":   "sha1",
		"SHA-256": "sha256",
		"SHA-384": "sha384",
		"SHA-512": "sha512",
	}[hashAlgorithm]
	if !ok {
		err = ErrUnsupportedHashAlgorithm
		return
	}
	var b bytes.Buffer
	s, _ := randomBytes(16)
	if salt != "" {
		if s, err = base64.StdEncoding.DecodeString(salt); err != nil {
			return
		}
	}
	b.Write(s)
	encoder := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewEncoder()
	passwordBuffer, _ := encoder.Bytes([]byte(passwd))
	b.Write(passwordBuffer)
	// Generate the initial hash.
	key := hashing(algorithmName, b.Bytes())
	// Now regenerate until spin count.
	for i := 0; i < spinCount; i++ {
		iterator := createUInt32LEBuffer(i, 4)
		key = hashing(algorithmName, key, iterator)
	}
	hashValue, saltValue = base64.StdEncoding.EncodeToString(key), base64.StdEncoding.EncodeToString(s)
	return
}

// Compound File Binary Implements

// cfb structure is used for the compound file binary (CFB) file format writer.
type cfb struct {
	stream   []byte
	position int
	paths    []string
	sectors  []sector
}

// sector structure used for FAT, directory, miniFAT, and miniStream sectors.
type sector struct {
	clsID, content                             []byte
	name                                       string
	C, L, R, color, size, start, state, typeID int
}

// writeBytes write bytes in the stream by a given value with an offset.
func (c *cfb) writeBytes(value []byte) {
	pos := c.position
	for i := 0; i < len(value); i++ {
		for j := len(c.stream); j <= i+pos; j++ {
			c.stream = append(c.stream, 0)
		}
		c.stream[i+pos] = value[i]
	}
	c.position = pos + len(value)
}

// writeUint16 write an uint16 data type bytes in the stream by a given value
// with an offset.
func (c *cfb) writeUint16(value int) {
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, uint16(value))
	c.writeBytes(buf)
}

// writeUint32 write an uint32 data type bytes in the stream by a given value
// with an offset.
func (c *cfb) writeUint32(value int) {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(value))
	c.writeBytes(buf)
}

// writeUint64 write an uint64 data type bytes in the stream by a given value
// with an offset.
func (c *cfb) writeUint64(value int) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(value))
	c.writeBytes(buf)
}

// writeBytes write strings in the stream by a given value with an offset.
func (c *cfb) writeStrings(value string) {
	encoder := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewEncoder()
	buffer, err := encoder.Bytes([]byte(value))
	if err != nil {
		return
	}
	c.writeBytes(buffer)
}

// put provides a function to add an entry to compound file by given entry name
// and raw bytes.
func (c *cfb) put(name string, content []byte) {
	path := c.paths[0]
	if len(path) <= len(name) && name[:len(path)] == path {
		path = name
	} else {
		if len(path) > 0 && string(path[len(path)-1]) != "/" {
			path += "/"
		}
		path = strings.ReplaceAll(path+name, "//", "/")
	}
	file := sector{name: path, typeID: 2, content: content, size: len(content)}
	c.sectors = append(c.sectors, file)
	c.paths = append(c.paths, path)
}

// compare provides a function to compare object path, each set of sibling
// objects in one level of the containment hierarchy (all child objects under
// a storage object) is represented as a red-black tree. The parent object of
// this set of siblings will have a pointer to the top of this tree.
func (c *cfb) compare(left, right string) int {
	L, R, i, j := strings.Split(left, "/"), strings.Split(right, "/"), 0, 0
	for Z := int(math.Min(float64(len(L)), float64(len(R)))); i < Z; i++ {
		if j = len(L[i]) - len(R[i]); j != 0 {
			return j
		}
		if L[i] != R[i] {
			if L[i] < R[i] {
				return -1
			}
			return 1
		}
	}
	return len(L) - len(R)
}

// prepare provides a function to prepare object before write stream.
func (c *cfb) prepare() {
	type object struct {
		path   string
		sector sector
	}
	var objects []object
	for i := 0; i < len(c.paths); i++ {
		if c.sectors[i].typeID == 0 {
			continue
		}
		objects = append(objects, object{path: c.paths[i], sector: c.sectors[i]})
	}
	sort.Slice(objects, func(i, j int) bool {
		return c.compare(objects[i].path, objects[j].path) == 0
	})
	c.paths, c.sectors = []string{}, []sector{}
	for i := 0; i < len(objects); i++ {
		c.paths = append(c.paths, objects[i].path)
		c.sectors = append(c.sectors, objects[i].sector)
	}
	for i := 0; i < len(objects); i++ {
		sector, path := &c.sectors[i], c.paths[i]
		sector.name, sector.color = filepath.Base(path), 1
		sector.L, sector.R, sector.C = -1, -1, -1
		sector.size, sector.start = len(sector.content), 0
		if len(sector.clsID) == 0 {
			sector.clsID = headerCLSID
		}
		if i == 0 {
			sector.C = -1
			if len(objects) > 1 {
				sector.C = 1
			}
			sector.size, sector.typeID = 0, 5
		} else {
			if len(c.paths) > i+1 && filepath.Dir(c.paths[i+1]) == filepath.Dir(path) {
				sector.R = i + 1
			}
			sector.typeID = 2
		}
	}
}

// locate provides a function to locate sectors location and size of the
// compound file.
func (c *cfb) locate() []int {
	var miniStreamSectorSize, FATSectorSize int
	for i := 0; i < len(c.sectors); i++ {
		sector := c.sectors[i]
		if len(sector.content) == 0 {
			continue
		}
		size := len(sector.content)
		if size > 0 {
			if size < 0x1000 {
				miniStreamSectorSize += (size + 0x3F) >> 6
			} else {
				FATSectorSize += (size + 0x01FF) >> 9
			}
		}
	}
	directorySectors := (len(c.paths) + 3) >> 2
	miniStreamSectors := (miniStreamSectorSize + 7) >> 3
	miniFATSectors := (miniStreamSectorSize + 0x7F) >> 7
	sectors := miniStreamSectors + FATSectorSize + directorySectors + miniFATSectors
	FATSectors := (sectors + 0x7F) >> 7
	DIFATSectors := 0
	if FATSectors > 109 {
		DIFATSectors = int(math.Ceil((float64(FATSectors) - 109) / 0x7F))
	}
	for ((sectors + FATSectors + DIFATSectors + 0x7F) >> 7) > FATSectors {
		FATSectors++
		if FATSectors <= 109 {
			DIFATSectors = 0
		} else {
			DIFATSectors = int(math.Ceil((float64(FATSectors) - 109) / 0x7F))
		}
	}
	location := []int{1, DIFATSectors, FATSectors, miniFATSectors, directorySectors, FATSectorSize, miniStreamSectorSize, 0}
	c.sectors[0].size = miniStreamSectorSize << 6
	c.sectors[0].start = location[0] + location[1] + location[2] + location[3] + location[4] + location[5]
	location[7] = c.sectors[0].start + ((location[6] + 7) >> 3)
	return location
}

// writeMSAT provides a function to write compound file master sector allocation
// table.
func (c *cfb) writeMSAT(location []int) {
	var i, offset int
	for i = 0; i < 109; i++ {
		if i < location[2] {
			c.writeUint32(location[1] + i)
		} else {
			c.writeUint32(-1)
		}
	}
	if location[1] != 0 {
		for offset = 0; offset < location[1]; offset++ {
			for ; i < 236+offset*127; i++ {
				if i < location[2] {
					c.writeUint32(location[1] + i)
				} else {
					c.writeUint32(-1)
				}
			}
			if offset == location[1]-1 {
				c.writeUint32(endOfChain)
			} else {
				c.writeUint32(offset + 1)
			}
		}
	}
}

// writeDirectoryEntry provides a function to write compound file directory
// entries. The directory entry array is an array of directory entries that
// are grouped into a directory sector. Each storage object or stream object
// within a compound file is represented by a single directory entry. The
// space for the directory sectors that are holding the array is allocated
// from the FAT.
func (c *cfb) writeDirectoryEntry(location []int) {
	var sector sector
	var j, sectorSize int
	for i := 0; i < location[4]<<2; i++ {
		var path string
		if i < len(c.paths) {
			path = c.paths[i]
		}
		if i >= len(c.paths) || len(path) == 0 {
			for j = 0; j < 17; j++ {
				c.writeUint32(0)
			}
			for j = 0; j < 3; j++ {
				c.writeUint32(-1)
			}
			for j = 0; j < 12; j++ {
				c.writeUint32(0)
			}
			continue
		}
		sector = c.sectors[i]
		if i == 0 {
			if sector.size > 0 {
				sector.start = sector.start - 1
			} else {
				sector.start = endOfChain
			}
		}
		name := sector.name
		sectorSize = 2 * (len(name) + 1)
		c.writeStrings(name)
		c.position += 64 - 2*(len(name))
		c.writeUint16(sectorSize)
		c.writeBytes([]byte(string(rune(sector.typeID))))
		c.writeBytes([]byte(string(rune(sector.color))))
		c.writeUint32(sector.L)
		c.writeUint32(sector.R)
		c.writeUint32(sector.C)
		if len(sector.clsID) == 0 {
			for j = 0; j < 4; j++ {
				c.writeUint32(0)
			}
		} else {
			c.writeBytes(sector.clsID)
		}
		c.writeUint32(sector.state)
		c.writeUint32(0)
		c.writeUint32(0)
		c.writeUint32(0)
		c.writeUint32(0)
		c.writeUint32(sector.start)
		c.writeUint32(sector.size)
		c.writeUint32(0)
	}
}

// writeSectorChains provides a function to write compound file sector chains.
func (c *cfb) writeSectorChains(location []int) sector {
	var i, j, offset, sectorSize int
	writeSectorChain := func(head, offset int) int {
		for offset += head; i < offset-1; i++ {
			c.writeUint32(i + 1)
		}
		if head != 0 {
			i++
			c.writeUint32(endOfChain)
		}
		return offset
	}
	for offset += location[1]; i < offset; i++ {
		c.writeUint32(difSect)
	}
	for offset += location[2]; i < offset; i++ {
		c.writeUint32(fatSect)
	}
	offset = writeSectorChain(location[3], offset)
	offset = writeSectorChain(location[4], offset)
	sector := c.sectors[0]
	for ; j < len(c.sectors); j++ {
		if sector = c.sectors[j]; len(sector.content) == 0 {
			continue
		}
		if sectorSize = len(sector.content); sectorSize < 0x1000 {
			continue
		}
		c.sectors[j].start = offset
		offset = writeSectorChain((sectorSize+0x01FF)>>9, offset)
	}
	writeSectorChain((location[6]+7)>>3, offset)
	for c.position&0x1FF != 0 {
		c.writeUint32(endOfChain)
	}
	i, offset = 0, 0
	for j = 0; j < len(c.sectors); j++ {
		if sector = c.sectors[j]; len(sector.content) == 0 {
			continue
		}
		if sectorSize = len(sector.content); sectorSize == 0 || sectorSize >= 0x1000 {
			continue
		}
		sector.start = offset
		offset = writeSectorChain((sectorSize+0x3F)>>6, offset)
	}
	for c.position&0x1FF != 0 {
		c.writeUint32(endOfChain)
	}
	return sector
}

// write provides a function to create compound file package stream.
func (c *cfb) write() []byte {
	c.prepare()
	location := c.locate()
	c.stream = make([]byte, location[7]<<9)
	var i, j int
	for i = 0; i < 8; i++ {
		c.writeBytes([]byte{oleIdentifier[i]})
	}
	c.writeBytes(make([]byte, 16))
	c.writeUint16(0x003E)
	c.writeUint16(0x0003)
	c.writeUint16(0xFFFE)
	c.writeUint16(0x0009)
	c.writeUint16(0x0006)
	c.writeBytes(make([]byte, 10))
	c.writeUint32(location[2])
	c.writeUint32(location[0] + location[1] + location[2] + location[3] - 1)
	c.writeUint32(0)
	c.writeUint32(1 << 12)
	if location[3] != 0 {
		c.writeUint32(location[0] + location[1] + location[2] - 1)
	} else {
		c.writeUint32(endOfChain)
	}
	c.writeUint32(location[3])
	if location[1] != 0 {
		c.writeUint32(location[0] - 1)
	} else {
		c.writeUint32(endOfChain)
	}
	c.writeUint32(location[1])
	c.writeMSAT(location)
	sector := c.writeSectorChains(location)
	c.writeDirectoryEntry(location)
	for i = 1; i < len(c.sectors); i++ {
		sector = c.sectors[i]
		if sector.size >= 0x1000 {
			c.position = (sector.start + 1) << 9
			for j = 0; j < sector.size; j++ {
				c.writeBytes([]byte{sector.content[j]})
			}
			for ; j&0x1FF != 0; j++ {
				c.writeBytes([]byte{0})
			}
		}
	}
	for i = 1; i < len(c.sectors); i++ {
		sector = c.sectors[i]
		if sector.size > 0 && sector.size < 0x1000 {
			for j = 0; j < sector.size; j++ {
				c.writeBytes([]byte{sector.content[j]})
			}
			for ; j&0x3F != 0; j++ {
				c.writeBytes([]byte{0})
			}
		}
	}
	for c.position < len(c.stream) {
		c.writeBytes([]byte{0})
	}
	return c.stream
}

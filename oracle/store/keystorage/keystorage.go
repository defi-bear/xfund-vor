package keystorage

import (
	"crypto/aes"
	"crypto/cipher"
	cryptoRand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
	"io"
	"io/ioutil"
	"math/rand"
	"oracle/models/keystorage"
	"oracle/utils"
	"oracle/utils/walletworker"
	"os"
	"time"
)

type Keystorage struct {
	log      *logrus.Logger
	File     *os.File
	KeyStore *keystorage.KeyStorageModel
}

func NewKeyStorage(log *logrus.Logger, filePath string) (*Keystorage, error) {
	var err error
	var keystoreFile *os.File
	var keyStore = keystorage.KeyStorageModel{}

	if _, err = os.Stat(filePath); err == nil {
		log.WithFields(logrus.Fields{
			"package":  "keystorage",
			"function": "NewKeyStorage",
			"action":   "reading file",
		}).Info()
		keystoreFile, err = os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0755)
		if err != nil {
			log.WithFields(logrus.Fields{
				"package":  "keystorage",
				"function": "NewKeyStorage",
				"action":   "reading file",
			}).Error(err.Error())
			return nil, err
		}

		data, err := ioutil.ReadAll(keystoreFile)
		if err != nil {
			log.WithFields(logrus.Fields{
				"package":  "keystorage",
				"function": "NewKeyStorage",
				"action":   "init KeyStore object",
			}).Error(err.Error())
			return nil, err
		}

		err = json.Unmarshal(data, &keyStore)
		if err != nil {
			log.WithFields(logrus.Fields{
				"package":  "keystorage",
				"function": "NewKeyStorage",
				"action":   "unmarshal json from file",
			}).Error(err.Error())
		}
	} else if os.IsNotExist(err) {
		keystoreFile, err = os.Create(filePath)
		_, err := keystoreFile.Write([]byte(`{"keys":[]}`))
		keyStore.Key = []*keystorage.KeyStorageKeyModel{}
		if err != nil {
			log.WithFields(logrus.Fields{
				"package":  "keystorage",
				"function": "NewKeyStorage",
				"action":   "creating file",
			}).Error(err.Error())
			return nil, err
		}

	}

	return &Keystorage{
		log:      log,
		File:     keystoreFile,
		KeyStore: &keyStore,
	}, err
}

func (d *Keystorage) GetFirst() *keystorage.KeyStorageKeyModel {
	key := d.KeyStore.GetKey()
	if key[0].Private == "" {
		key[0].Private, _ = Decrypt(key[0].CipherPrivate, d.KeyStore.Token)
	}
	d.KeyStore.Key = key
	return key[0]
}

func (d *Keystorage) GetByUsername(account string) *keystorage.KeyStorageKeyModel {
	keys := d.KeyStore.GetKey()
	for _, key := range keys {
		if key.Account == account {
			if key.Private == "" {
				key.Private, _ = Decrypt(key.CipherPrivate, d.KeyStore.Token)
			}
			d.KeyStore.Key = keys
			return key
		}
	}
	return &keystorage.KeyStorageKeyModel{}
}

func (d *Keystorage) ExistsByUsername(account string) bool {
	keys := d.KeyStore.GetKey()
	for _, key := range keys {
		if key.Account == account {
			return true
		}
	}
	d.KeyStore.Key = keys
	return false
}

func (d *Keystorage) GeneratePrivate(username string) (string, error) {
	_, keyGeneratedString, err := walletworker.GeneratePrivate()
	if err != nil {
		return "", err
	}
	cipherPrivate, err := Encrypt(keyGeneratedString, d.KeyStore.Token)
	if err != nil {
		return "", err
	}
	var newKey = append(d.KeyStore.Key, &keystorage.KeyStorageKeyModel{
		Account: func() string {
			if username == "" {
				return "autogenerated"
			}
			return username
		}(),
		CipherPrivate: cipherPrivate,
		Private:       keyGeneratedString,
	})
	d.KeyStore.Key = newKey
	err = d.save()
	return keyGeneratedString, err
}

func (d *Keystorage) AddExisting(username string, privateKey string) (err error) {
	privateKey = utils.AddHexPrefix(privateKey)

	cipherPrivate, err := Encrypt(privateKey, d.KeyStore.Token)
	if err != nil {
		return err
	}
	var newKey = append(d.KeyStore.Key, &keystorage.KeyStorageKeyModel{
		Account: func() string {
			if username == "" {
				return "autogenerated"
			}
			return username
		}(),
		CipherPrivate: cipherPrivate,
		Private:       privateKey,
	})
	d.KeyStore.Key = newKey
	err = d.save()
	return
}

func (d Keystorage) GetByAccount(account string) (*keystorage.KeyStorageKeyModel, error) {
	var keys = d.KeyStore.GetKey()
	for _, key := range keys {
		if key.Account == account {
			key.Private, _ = Decrypt(key.CipherPrivate, d.KeyStore.Token)
			return key, nil
		}
	}
	return &keystorage.KeyStorageKeyModel{}, fmt.Errorf("Can't find user, sorry.")
}

func (d *Keystorage) SetRegistered(privateKey string) (err error) {
	keys := d.KeyStore.GetKey()

	for index, key := range keys {
		if decryptedPrivate, _ := Decrypt(key.CipherPrivate, d.KeyStore.Token); decryptedPrivate == privateKey {
			d.KeyStore.Key[index].Registered = true
			err = d.save()
			return
		}
	}
	return
}

func (d *Keystorage) SetBlockNumber(blockNumber int64) (err error) {
	keys := d.KeyStore.GetKey()

	for index, key := range keys {
		if decryptedPrivate, _ := Decrypt(key.CipherPrivate, d.KeyStore.Token); decryptedPrivate == d.KeyStore.PrivateKey {
			d.KeyStore.Key[index].BlockNumber = blockNumber
			err = d.save()
			return
		}
	}
	return
}

func (d *Keystorage) GetBlockNumber() (blockNumber int64, err error) {
	keys := d.KeyStore.GetKey()

	for index, key := range keys {
		if decryptedPrivate, _ := Decrypt(key.CipherPrivate, d.KeyStore.Token); decryptedPrivate == d.KeyStore.PrivateKey {
			blockNumber = d.KeyStore.Key[index].BlockNumber
			err = d.save()
			return
		}
	}
	return
}

func (d *Keystorage) IsRegisteredByPrivate(privateKey string) (registered bool) {
	keys := d.KeyStore.GetKey()
	for _, key := range keys {
		if decryptedPrivate, _ := Decrypt(key.CipherPrivate, d.KeyStore.Token); decryptedPrivate == privateKey {
			return key.GetRegistered()
		}
	}

	return false
}

func (d Keystorage) Exists() bool {
	if len((d.KeyStore.GetKey())) > 0 {
		return true
	}

	return false
}

func (d *Keystorage) save() error {
	jsonByte, err := json.Marshal(d.KeyStore)
	if err != nil {
		return err
	}
	_, err = d.File.WriteAt(jsonByte, 0)
	return err
}

func (d *Keystorage) GenerateToken() (string, error) {
	rand.Seed(time.Now().Unix())
	d.KeyStore.Token = GenerateRandomBytes(32)
	err := d.tokenEncryptAndSave()
	return d.KeyStore.Token, err
}

func (d *Keystorage) CheckToken(token string) (err error) {
	err = bcrypt.CompareHashAndPassword([]byte(d.KeyStore.Hash), []byte(token))
	if err == nil {
		d.KeyStore.Token = token
	}
	return
}

func (d *Keystorage) SelectPrivateKey(account string) (err error) {
	if pKey, err := d.GetByAccount(account); err == nil {
		d.KeyStore.PrivateKey = pKey.GetPrivate()
	} else {
		d.KeyStore.PrivateKey = d.GetFirst().GetPrivate()
	}
	return err
}

func (d *Keystorage) GetSelectedPrivateKey() string {
	return d.KeyStore.GetPrivateKey()
}

func (d *Keystorage) tokenEncryptAndSave() (err error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(d.KeyStore.Token), 8)
	if err != nil {
		return
	}
	d.KeyStore.Hash = string(hash)
	err = d.save()
	return
}

// Takes two strings, cryptoText and keyString.
// cryptoText is the text to be decrypted and the keyString is the key to use for the decryption.
// The function will output the resulting plain text string with an error variable.
func Decrypt(cryptoText string, keyString string) (plainTextString string, err error) {

	encrypted, err := base64.URLEncoding.DecodeString(cryptoText)
	if err != nil {
		return "", err
	}
	if len(encrypted) < aes.BlockSize {
		return "", fmt.Errorf("cipherText too short. It decodes to %v bytes but the minimum length is 16", len(encrypted))
	}

	decrypted, err := decryptAES(hashTo32Bytes(keyString), encrypted)
	if err != nil {
		return "", err
	}

	return string(decrypted), nil
}

func decryptAES(key, data []byte) ([]byte, error) {
	// split the input up in to the IV seed and then the actual encrypted data.
	iv := data[:aes.BlockSize]
	data = data[aes.BlockSize:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	stream := cipher.NewCFBDecrypter(block, iv)

	stream.XORKeyStream(data, data)
	return data, nil
}

// Takes two string, plainText and keyString.
// plainText is the text that needs to be encrypted by keyString.
// The function will output the resulting crypto text and an error variable.
func Encrypt(plainText string, keyString string) (cipherTextString string, err error) {

	key := hashTo32Bytes(keyString)
	encrypted, err := encryptAES(key, []byte(plainText))
	if err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(encrypted), nil
}

func encryptAES(key, data []byte) ([]byte, error) {

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// create two 'windows' in to the output slice.
	output := make([]byte, aes.BlockSize+len(data))
	iv := output[:aes.BlockSize]
	encrypted := output[aes.BlockSize:]

	// populate the IV slice with random data.
	if _, err = io.ReadFull(cryptoRand.Reader, iv); err != nil {
		return nil, err
	}

	stream := cipher.NewCFBEncrypter(block, iv)

	// note that encrypted is still a window in to the output slice
	stream.XORKeyStream(encrypted, data)
	return output, nil
}

// hashTo32Bytes will compute a cryptographically useful hash of the input string.
func hashTo32Bytes(input string) []byte {

	data := sha256.Sum256([]byte(input))
	return data[0:]

}

// GenerateRandomBytes returns securely generated random bytes.
// It will return an error if the system's secure random
// number generator fails to function correctly, in which
// case the caller should not continue.
func GenerateRandomBytes(strSize int) string {

	var dictionary = "0123456789abcdefghijklmnopqrstuvwxyz"

	var bytes = make([]byte, strSize)
	rand.Read(bytes)
	for k, v := range bytes {
		bytes[k] = dictionary[v%byte(len(dictionary))]
	}
	return string(bytes)
}

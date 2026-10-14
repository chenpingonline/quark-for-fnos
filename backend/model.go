package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type File struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Folder     bool   `json:"folder"`
	Size       int64  `json:"size"`
	ModifiedAt int64  `json:"modified_at,omitempty"`
	Parent     string `json:"parent,omitempty"`
	Token      string `json:"token,omitempty"`
}
type FilePage struct {
	Files []File `json:"files"`
	Total int    `json:"total"`
	Page  int    `json:"page"`
}
type Settings struct {
	DownloadDir string `json:"download_dir"`
	Concurrent  int    `json:"concurrent"`
	Connections int    `json:"connections"`
	ChunkMiB    int    `json:"chunk_mib"`
	LimitMiB    int    `json:"limit_mib"`
	ReserveGiB  int    `json:"reserve_gib"`
	Retries     int    `json:"retries"`
	Cleanup     bool   `json:"cleanup"`
}
type Chunk struct {
	Start  int64  `json:"start"`
	End    int64  `json:"end"`
	Done   bool   `json:"done"`
	SHA256 string `json:"sha256,omitempty"`
}
type Task struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	Name           string    `json:"name"`
	RemoteID       string    `json:"remote_id"`
	ShareID        string    `json:"share_id,omitempty"`
	ShareToken     string    `json:"share_token,omitempty"`
	ShareFileToken string    `json:"share_file_token,omitempty"`
	ShareParent    string    `json:"share_parent,omitempty"`
	SavedID        string    `json:"saved_id,omitempty"`
	TempFolderID   string    `json:"temp_folder_id,omitempty"`
	Relative       string    `json:"relative"`
	Destination    string    `json:"destination"`
	Size           int64     `json:"size"`
	Downloaded     int64     `json:"downloaded"`
	Status         string    `json:"status"`
	Message        string    `json:"message,omitempty"`
	Speed          int64     `json:"speed"`
	SHA256         string    `json:"sha256,omitempty"`
	ETag           string    `json:"etag,omitempty"`
	LastModified   string    `json:"last_modified,omitempty"`
	Chunks         []Chunk   `json:"chunks,omitempty"`
	Updated        time.Time `json:"updated"`
	Created        time.Time `json:"created"`
	Attempts       int       `json:"attempts"`
}
type QRSession struct {
	ID     string    `json:"id"`
	URL    string    `json:"url"`
	Status string    `json:"status"`
	Expire time.Time `json:"expire"`
	token  string
}
type SharePreview struct {
	ID      string    `json:"id"`
	ShareID string    `json:"share_id"`
	Token   string    `json:"-"`
	Title   string    `json:"title"`
	Expire  time.Time `json:"expire"`
}
type Persistent struct {
	Tasks           []*Task  `json:"tasks"`
	Settings        Settings `json:"settings"`
	CookieEncrypted string   `json:"cookie_encrypted,omitempty"`
	TempFolderID    string   `json:"temp_folder_id,omitempty"`
}
type Store struct {
	mu    sync.Mutex
	state Persistent
	path  string
	key   []byte
}

func defaults() Settings {
	return Settings{Concurrent: 3, Connections: 4, ChunkMiB: 8, ReserveGiB: 20, Retries: 5, Cleanup: true}
}
func makeID() string { var b [16]byte; _, _ = rand.Read(b[:]); return fmt.Sprintf("%x", b[:]) }
func newStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "state.json")}
	s.state.Settings = defaults()
	keypath := filepath.Join(dir, "credential.key")
	key, err := os.ReadFile(keypath)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		if err = writeAtomic(keypath, key, 0600); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("credential.key 长度非法")
	}
	s.key = key
	b, err := os.ReadFile(s.path)
	if err == nil {
		if err = json.Unmarshal(b, &s.state); err != nil {
			return nil, fmt.Errorf("state.json 无法解析（未覆盖原数据）: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if s.state.Tasks == nil {
		s.state.Tasks = []*Task{}
	}
	for _, task := range s.state.Tasks {
		if task.ShareToken, err = s.unseal(task.ShareToken); err != nil {
			return nil, err
		}
		if task.ShareFileToken, err = s.unseal(task.ShareFileToken); err != nil {
			return nil, err
		}
	}
	return s, nil
}
func writeAtomic(path string, b []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, e := os.CreateTemp(dir, ".quarkdrive-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(mode); e != nil {
		f.Close()
		return e
	}
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(f.Name(), path); e != nil {
		return e
	}
	d, e := os.Open(dir)
	if e == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// Do not leave share access tokens in plaintext task checkpoints on disk.
const secretPrefix = "sealed:v1:"

func (s *Store) seal(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return secretPrefix + base64.StdEncoding.EncodeToString(g.Seal(nonce, nonce, []byte(value), nil)), nil
}
func (s *Store) unseal(value string) (string, error) {
	if !strings.HasPrefix(value, secretPrefix) {
		return value, nil
	} // Old local checkpoints
	encoded := strings.TrimPrefix(value, secretPrefix)
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < g.NonceSize() {
		return "", errors.New("加密任务 token 已损坏")
	}
	plain, err := g.Open(nil, data[:g.NonceSize()], data[g.NonceSize():], nil)
	return string(plain), err
}

func (s *Store) save() error {
	// Shallow-copy the top level and individually clone tasks to leave in-memory
	// tokens usable while protecting serialized checkpoints.
	onDisk := s.state
	onDisk.Tasks = make([]*Task, len(s.state.Tasks))
	for i, task := range s.state.Tasks {
		copy := *task
		var err error
		copy.ShareToken, err = s.seal(task.ShareToken)
		if err != nil {
			return err
		}
		copy.ShareFileToken, err = s.seal(task.ShareFileToken)
		if err != nil {
			return err
		}
		onDisk.Tasks[i] = &copy
	}
	b, err := json.MarshalIndent(onDisk, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.path, b, 0600)
}
func (s *Store) mutate(fn func(*Persistent) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fn(&s.state); err != nil {
		return err
	}
	return s.save()
}
func (s *Store) snapshot() Persistent {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(s.state)
	var cp Persistent
	_ = json.Unmarshal(b, &cp)
	return cp
}
func (s *Store) credential() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.CookieEncrypted == "" {
		return "", nil
	}
	data, e := base64.StdEncoding.DecodeString(s.state.CookieEncrypted)
	if e != nil {
		return "", e
	}
	block, e := aes.NewCipher(s.key)
	if e != nil {
		return "", e
	}
	aead, e := cipher.NewGCM(block)
	if e != nil {
		return "", e
	}
	if len(data) < aead.NonceSize() {
		return "", errors.New("密文格式无效")
	}
	plain, e := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], nil)
	return string(plain), e
}
func (s *Store) setCredential(cookie string) error {
	return s.mutate(func(p *Persistent) error {
		if cookie == "" {
			p.CookieEncrypted = ""
			return nil
		}
		block, e := aes.NewCipher(s.key)
		if e != nil {
			return e
		}
		g, e := cipher.NewGCM(block)
		if e != nil {
			return e
		}
		nonce := make([]byte, g.NonceSize())
		if _, e = rand.Read(nonce); e != nil {
			return e
		}
		p.CookieEncrypted = base64.StdEncoding.EncodeToString(g.Seal(nonce, nonce, []byte(cookie), nil))
		return nil
	})
}
func (s *Store) task(id string) *Task {
	p := s.snapshot()
	for _, t := range p.Tasks {
		if t.ID == id {
			return t
		}
	}
	return nil
}
func (s *Store) setTask(id string, fn func(*Task)) error {
	return s.mutate(func(p *Persistent) error {
		for _, t := range p.Tasks {
			if t.ID == id {
				fn(t)
				t.Updated = time.Now()
				return nil
			}
		}
		return os.ErrNotExist
	})
}

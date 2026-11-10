//go:build managedtrial

package managedtrial

import "time"

// Local's sync-replay store: the frames each channel published, so a tab that
// reconnects with a Last-Event-ID is handed what it missed instead of a whole
// new document. Nothing here is durable — the window is five minutes of frames
// and two hard caps — and a resume older than what the store still holds answers
// `resync`, which is the client's signal to reload the document.
//
// A frame is retained per (file, lane) bucket, keyed the way a channel is, and
// the ids are the hub's own seq numbers, so a client's lastEventId means the
// same thing to both.

const (
	replayBucketMaxFrames = 64
	replayBucketMaxBytes  = 16 * 1024 * 1024
	replayGlobalMaxFrames = 512
	replayGlobalMaxBytes  = 64 * 1024 * 1024
	replayMaxIdleBuckets  = 1024
)

// replayTTL is REPLAY_TTL_MS. It is a variable because a unit test wants a window
// it can spend.
var replayTTL = 5 * time.Minute

var replayLanes = map[string]bool{"live": true, "saved": true}

type replayFrame struct {
	seq     int64
	message string
	bytes   int
	at      int64
}

type replayBucket struct {
	frames         []replayFrame
	bytes          int
	droppedThrough int64
	createdSeq     int64
}

// replayStore is Local's ReplayStore. now is Date.now, replaced by a pinned
// clock in a golden.
type replayStore struct {
	now     func() int64
	buckets map[string]*replayBucket
	frames  int
	bytes   int
	lastSeq int64
}

// replay is Local's one store, started with frame ids on and the hub's frames as
// its tap.
var replay = newReplayStore()

func newReplayStore() *replayStore {
	store := &replayStore{
		now:     func() int64 { return liveNow() },
		buckets: make(map[string]*replayBucket),
	}
	store.lastSeq = store.now()
	return store
}

func replayKey(file, lane string) string { return lane + "\n" + file }

func (s *replayStore) bucket(file, lane string) *replayBucket {
	key := replayKey(file, lane)
	bucket, ok := s.buckets[key]
	if !ok {
		bucket = &replayBucket{createdSeq: s.lastSeq}
		s.buckets[key] = bucket
		s.reapIdle()
	}
	return bucket
}

// record is the frame tap: it keeps one published frame, if its lane is one this
// store holds.
func (s *replayStore) record(frame liveFrame) {
	if frame.Seq > s.lastSeq {
		s.lastSeq = frame.Seq
	}
	if !replayLanes[frame.Lane] {
		return
	}
	bucket := s.bucket(frame.File, frame.Lane)
	size := len(frame.Message)
	if size > replayBucketMaxBytes {
		if frame.Seq > bucket.droppedThrough {
			bucket.droppedThrough = frame.Seq
		}
		return
	}
	bucket.frames = append(bucket.frames, replayFrame{seq: frame.Seq, message: frame.Message, bytes: size, at: s.now()})
	bucket.bytes += size
	s.frames++
	s.bytes += size
	for len(bucket.frames) > replayBucketMaxFrames || bucket.bytes > replayBucketMaxBytes {
		s.dropOldest(bucket)
	}
	for s.frames > replayGlobalMaxFrames || s.bytes > replayGlobalMaxBytes {
		var oldest *replayBucket
		for _, candidate := range s.buckets {
			if len(candidate.frames) == 0 {
				continue
			}
			if oldest == nil || candidate.frames[0].seq < oldest.frames[0].seq {
				oldest = candidate
			}
		}
		if oldest == nil {
			break
		}
		s.dropOldest(oldest)
	}
}

func (s *replayStore) dropOldest(bucket *replayBucket) {
	frame := bucket.frames[0]
	bucket.frames = bucket.frames[1:]
	bucket.bytes -= frame.bytes
	s.frames--
	s.bytes -= frame.bytes
	if frame.seq > bucket.droppedThrough {
		bucket.droppedThrough = frame.seq
	}
}

func (s *replayStore) expire() {
	cutoff := s.now() - replayTTL.Milliseconds()
	for _, bucket := range s.buckets {
		for len(bucket.frames) > 0 && bucket.frames[0].at < cutoff {
			s.dropOldest(bucket)
		}
	}
}

func (s *replayStore) reapIdle() {
	idle := 0
	for _, bucket := range s.buckets {
		if len(bucket.frames) == 0 {
			idle++
		}
	}
	if idle <= replayMaxIdleBuckets {
		return
	}
	for key, bucket := range s.buckets {
		if idle <= replayMaxIdleBuckets {
			break
		}
		if len(bucket.frames) == 0 {
			delete(s.buckets, key)
			idle--
		}
	}
}

// replayAnswer is what a stream writes before subscribing: the position its
// frames continue from, the frames after `since`, and whether the client's
// position is older than anything the store still holds.
type replayAnswer struct {
	Baseline int64
	Replay   []string
	Resync   bool
}

func (s *replayStore) resume(file, lane string, since int64) replayAnswer {
	s.expire()
	bucket := s.bucket(file, lane)
	if since <= 0 || since > s.lastSeq {
		return replayAnswer{Baseline: s.lastSeq}
	}
	var frames []string
	for _, frame := range bucket.frames {
		if frame.seq > since {
			frames = append(frames, frame.message)
		}
	}
	resync := since < bucket.createdSeq || since < bucket.droppedThrough
	return replayAnswer{Baseline: since, Replay: frames, Resync: resync}
}

//go:build darwin && cgo

package carried

/*
#include <libproc.h>
#include <sys/resource.h>

static long long carried_footprint(int pid) {
	struct rusage_info_v2 ri;
	if (proc_pid_rusage(pid, RUSAGE_INFO_V2, (rusage_info_t *)&ri) != 0) {
		return -1;
	}
	return (long long)ri.ri_phys_footprint;
}
*/
import "C"

const memoryWatched = true

// memoryOf is the process's physical footprint, which counts compressed
// memory too, so a program cannot hide its size behind the compressor.
func memoryOf(pid int) (uint64, bool) {
	n := C.carried_footprint(C.int(pid))
	if n < 0 {
		return 0, false
	}
	return uint64(n), true
}

//go:build windows

package procrun

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var trackedJobs sync.Map // map[*exec.Cmd]windows.Handle
var ntResumeProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")

func configurePlatform(cmd *exec.Cmd) {
	cmd.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

func configureTrackedPlatform(cmd *exec.Cmd) {
	cmd.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED}
}

func KillTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pid := uint32(cmd.Process.Pid)
	if value, ok := trackedJobs.LoadAndDelete(cmd); ok {
		job := value.(windows.Handle)
		err := windows.TerminateJobObject(job, 1)
		_ = windows.CloseHandle(job)
		if err == nil {
			return nil
		}
	}
	if err := killDescendants(pid); err != nil {
		_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	}
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func trackTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return errors.New("process has not started")
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("create job object: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("configure job object: %w", err)
	}
	processHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_SUSPEND_RESUME,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("open process for job: %w", err)
	}
	defer windows.CloseHandle(processHandle)
	if err := windows.AssignProcessToJobObject(job, processHandle); err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("assign process to job: %w", err)
	}
	trackedJobs.Store(cmd, job)
	if status, _, _ := ntResumeProcess.Call(uintptr(processHandle)); int32(status) < 0 {
		_ = KillTree(cmd)
		return fmt.Errorf("resume process: NTSTATUS 0x%08x", uint32(status))
	}
	return nil
}

func releaseTree(cmd *exec.Cmd) error {
	value, ok := trackedJobs.LoadAndDelete(cmd)
	if !ok {
		return nil
	}
	return windows.CloseHandle(value.(windows.Handle))
}

func killDescendants(root uint32) error {
	children, err := processChildren()
	if err != nil {
		return err
	}
	var errs []error
	for _, pid := range postorder(root, children) {
		if err := terminateProcess(pid); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func processChildren() (map[uint32][]uint32, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)

	children := map[uint32][]uint32{}
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err := windows.Process32First(snap, &pe); err != nil {
		return nil, err
	}
	for {
		children[pe.ParentProcessID] = append(children[pe.ParentProcessID], pe.ProcessID)
		if err := windows.Process32Next(snap, &pe); err != nil {
			if err == windows.ERROR_NO_MORE_FILES {
				break
			}
			return nil, err
		}
	}
	return children, nil
}

func postorder(root uint32, children map[uint32][]uint32) []uint32 {
	var out []uint32
	var walk func(uint32)
	walk = func(pid uint32) {
		for _, child := range children[pid] {
			walk(child)
		}
		if pid != root {
			out = append(out, pid)
		}
	}
	walk(root)
	return out
}

func terminateProcess(pid uint32) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		if err == windows.ERROR_INVALID_PARAMETER {
			return nil
		}
		return fmt.Errorf("open process %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)
	if err := windows.TerminateProcess(h, 1); err != nil {
		return fmt.Errorf("terminate process %d: %w", pid, err)
	}
	event, err := windows.WaitForSingleObject(h, 500)
	if err != nil {
		return fmt.Errorf("wait for process %d: %w", pid, err)
	}
	if event == uint32(windows.WAIT_TIMEOUT) {
		return fmt.Errorf("wait for process %d: timeout", pid)
	}
	return nil
}

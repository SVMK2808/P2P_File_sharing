#include <iostream>
#include <string>
#include <unistd.h>
#include <fstream>
#if defined(__linux__)
#define OS_LINUX
#include <sys/stat.h>
#include <fcntl.h>
#include <cstring>
#include <fstream>
#elif defined(__APPLE__) && defined(__MACH__)
#define OS_MAC
#include <sys/sysctl.h>
#include <mach/mach.h>
#include <libproc.h>
#include <pwd.h>
#else
#error "Unsupported OS"
#endif

void pinfo_linux(pid_t pid)
{
    std::string pid_str = std::to_string(pid);
    std::string status_path = "/proc/" + pid_str + "/status";
    std::string statm_path = "/proc/" + pid_str + "/statm";
    std::string exe_path = "/proc/" + pid_str + "/exe";

    std::ifstream status_file(status_path);
    std::ifstream statm_file(statm_path);

    if (!status_file)
    {
        std::cerr << "pinfo: Process with pid " << pid << "not found\n";
        return;
    }

    std::string line;
    std::string state;
    while (std::getline(status_file, line))
    {
        if (line.find("State:") == 0)
        {
            state = line.substr(0);
            break;
        }
    }

    std::string mem;
    if (statm_file >> mem)
    {
        std::cout << "memory usage : " << mem << "KB\n";
    }

    char exe[1024];
    ssize_t len = readlink(exe_path.c_str(), exe, sizeof(exe) - 1);
    if (len != -1)
        exe[len] = '\0';
    else
        strcpy(exe, "[exe path not available]");

    std::cout << "pid -- " << pid << "\n";
    std::cout << "process status -- " << state << "\n";
    std::cout << "memory -- " << mem << "\n";
    std::cout << "executable path -- " << exe << "\n";
}

void pinfo_mac(pid_t pid)
{
    struct kinfo_proc kp;
    size_t len = sizeof(kp);

    int mib[4] = {CTL_KERN, KERN_PROC, KERN_PROC_PID, pid};
    if (sysctl(mib, 4, &kp, &len, NULL, 0) == -1 || len == 0)
    {
        std::cerr << "pinfo : no such process with pid " << pid << "exists\n";
        return;
    }

    std::cout << "pid -- " << pid << "\n";
    std::cout << "status -- ";

    switch (kp.kp_proc.p_stat)
    {
    case SRUN:
        std::cout << "Running\n";
        break;
    case SSLEEP:
        std::cout << "Sleeping\n";
        break;
    case SZOMB:
        std::cout << "Zombie\n";
        break;
    case SSTOP:
        std::cout << "Stopped\n";
        break;
    default:
        std::cout << "Unknown\n";
        break;
    }

    mach_port_t task;
    kern_return_t kr = task_for_pid(mach_task_self(), pid, &task);
    if (kr != KERN_SUCCESS)
    {
        std::cerr << "task_for_pid failed : " << mach_error_string(kr) << "\n";
        return;
    }

    struct task_basic_info t_info;
    mach_msg_type_number_t t_info_count = TASK_BASIC_INFO_COUNT;
    task_info(task, TASK_BASIC_INFO, (task_info_t)&t_info, &t_info_count);
    if (kr != KERN_SUCCESS)
    {
        std::cerr << "task info failed : " << mach_error_string(kr) << "\n";
        return;
    }

    std::cout << "Memory -- " << t_info.resident_size << "bytes\n";
    char pathbuf[PROC_PIDPATHINFO_MAXSIZE];
    if (proc_pidpath(pid, pathbuf, sizeof(pathbuf)) > 0)
    {
        std::cout << "executable path -- " << pathbuf << "\n";
    }
    else
    {
        std::cerr << "Could not get executable path fpr the process with pid : " << pid << "\n";
    }
}

void pinfo_cross_platform(pid_t pid)
{
#ifdef OS_LINUX
    pinfo_linux(pid);
#elif defined(OS_MAC)
    pinfo_mac(pid);
#endif
}

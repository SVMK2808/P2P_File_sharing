#pragma once
#include <string>

class Shell
{
public:
    Shell();
    void run();
    void renderPrompt();
    void setFg_pid(pid_t pid);
    static void handle_sigtstp_wrapper(int signum);
    static void handle_sigint_wrapper(int signum);
    static void setShell_pgid(pid_t pgid);

private:
    std::string root_path;
    static pid_t fg_pid;
    static pid_t shell_pgid;
    static void handle_sigint(int signum);
    static void handle_sigtstp(int signum);
    void displayPrompt();
    std::string getPath();
    std::string getInput();
};
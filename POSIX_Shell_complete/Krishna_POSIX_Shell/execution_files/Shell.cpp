#include "Shell.h"
#include "Executor.h"
#include "Parser.h"
#include "Pipeliner.h"
#include "../header_files/Autocompleter.h"
#include "../header_files/historyCommands.h"
#include "Pinfo.h"
#include "SearchCommand.h"
#include "Echo.h"
#include "LSCommand.h"
#include <iostream>
#include <unistd.h>
#include <pwd.h>
#include <limits.h>
#include <termios.h>
#include <signal.h>
#include <stdlib.h>

termios orig_termios;
#define HOST_NAME_MAX 1024
pid_t Shell::fg_pid = -1;
pid_t Shell::shell_pgid = -1;

void disableRawMode()
{
    tcsetattr(STDIN_FILENO, TCSAFLUSH, &orig_termios);
}

void enableRawMode()
{
    tcgetattr(STDIN_FILENO, &orig_termios);
    atexit(disableRawMode);

    termios raw = orig_termios;
    raw.c_lflag &= ~(ECHO | ICANON);
    tcsetattr(STDIN_FILENO, TCSAFLUSH, &raw);
}

void Shell::handle_sigint_wrapper(int signum)
{
    handle_sigint(signum);
}

void Shell::handle_sigtstp_wrapper(int signum)
{
    handle_sigtstp(signum);
}

Shell::Shell()
{
    signal(SIGINT, handle_sigint_wrapper);
    signal(SIGTSTP, handle_sigtstp_wrapper);
    signal(SIGTTIN, SIG_IGN);
    signal(SIGTTOU, SIG_IGN);
}

void Shell::handle_sigint(int signum)
{
    if (fg_pid != -1)
    {
        kill(-fg_pid, SIGINT);
    }
    std::cout << "\n";
}

void Shell::handle_sigtstp(int signum)
{
    if (fg_pid != -1)
    {
        std::cout << "\nProcess: " << fg_pid << " suspended and pushed to background\n";
        std::cout.flush();
        kill(-fg_pid, SIGTSTP);
    }
}

void Shell::setFg_pid(pid_t pid)
{
    fg_pid = pid;
}

void Shell::setShell_pgid(pid_t pgid)
{
    shell_pgid = pgid;
}

void Shell::run()
{
    pid_t shl_pgid = getpid();
    setpgid(Shell::shell_pgid, shl_pgid);

    tcsetpgrp(STDIN_FILENO, shell_pgid);
    fg_pid = shl_pgid;

    root_path = getPath();
    enableRawMode();
    while (true)
    {
        displayPrompt();
        std::string input = getInput();
        if (input == "__EOF__" || input == "exit")
        {
            std::cout << "\nExiting Shell...\n";
            exit(EXIT_SUCCESS);
        }

        else if (input.empty())
            continue;

        std::vector<std::string> semiColonCommands = Parser::SemiColonSplit(input);

        for (const auto &fullCmd : semiColonCommands)
        {
            std::string trimmed = Parser::trim(fullCmd);

            bool isBackground = false;
            std::string cmdStr = trimmed;

            if (!cmdStr.empty() && cmdStr.back() == '&')
            {
                isBackground = true;
                cmdStr.pop_back();
                cmdStr = Parser::trim(cmdStr);
            }

            std::vector<std::string> pipes = Parser::PipeOprSplit(cmdStr);

            if (pipes.size() == 1)
            {
                // No pipe operator. Just a simple command
                std::vector<std::string> tokens = Parser::tokenizeCommand(pipes[0]);

                if (tokens[0] == "ls")
                {
                    ls(tokens.size(), tokens);
                }
                else if (tokens[0] == "search")
                {
                    handleSearchCommand(tokens);
                }
                else if (tokens[0] == "echo")
                {
                    // TODO: implement echo command function
                    echoCommand(tokens);
                }
                else if (tokens[0] == "pinfo")
                {
                    pid_t pid = 0;
                    if (tokens.size() == 1)
                    {
                        pid = getpid();
                    }
                    else
                    {
                        pid = stoi(tokens[1]);
                    }

                    pinfo_cross_platform(pid);
                }
                else if (tokens[0] == "history")
                {
                    handleHistoryCommand();
                }
                else
                    Executor::executeCommand(tokens, isBackground);
            }

            else
            {
                // TODO: Handle piping later
                Pipeliner::executePipelineCommands(pipes, isBackground);
            }
        }

        std::cout << "You entered: " << input << "\n";
        std::cout.flush();
    }
}

void Shell::displayPrompt()
{
    char hostname[HOST_NAME_MAX];
    char cwd[PATH_MAX];
    char *username = getpwuid(getuid())->pw_name;
    gethostname(hostname, sizeof(hostname));
    getcwd(cwd, sizeof(cwd));
    std::string cwdStr = cwd;
    cwdStr.erase(0, root_path.length());
    cwdStr.insert(cwdStr.begin(), '~');
    std::string disp_path = (cwdStr == root_path) ? "~" : cwdStr;
    std::cout << username << "@" << hostname << ":" << disp_path << ">";
    std::cout.flush();
}

void Shell::renderPrompt()
{
    displayPrompt();
}

std::string Shell::getInput()
{
    std::string input;
    char c;
    static std::vector<std::string> history;
    static int historyIndex = -1;

    while (true)
    {
        ssize_t n;

        n = read(STDIN_FILENO, &c, 1);

        if (n == -1)
            perror("read");

        if (c == 4)
        {
            history.clear();
            return "__EOF__";
        }

        else if (c == '\t')
        {
            Autocompleter autoComp;
            input = autoComp.handleTabCompletion(input, [this]()
                                                 { this->renderPrompt(); });
            continue;
        }

        else if (c == '\n')
        {
            std::cout << '\n';
            if (!input.empty())
            {
                saveToHistory(input);
                history.push_back(input);
                if (history.size() > 20)
                    history.erase(history.begin());
            }
            historyIndex = history.size();
            return input;
        }

        // Handle backspace
        else if (c == 127)
        {
            if (!input.empty())
            {
                input.pop_back();
                std::cout << "\b \b";
                std::cout.flush();
            }
        }

        // Handle Arrow keys
        else if (c == '\033')
        { // Escape sequence
            char seq[2];
            if (read(STDIN_FILENO, &seq[0], 1) == 0)
                continue;
            if (read(STDIN_FILENO, &seq[1], 1) == 0)
                continue;

            if (seq[0] == '[')
            {
                if (seq[1] == 'A')
                { // Up arrow key
                    if (historyIndex > 0)
                    {
                        historyIndex--;
                        input = history[historyIndex];
                        std::cout << "\33[2K\r"; // Clear the line
                        std::cout.flush();
                        displayPrompt();
                        std::cout << input;
                        std::cout.flush();
                    }
                }
                else if (seq[1] == 'B')
                { // Down arrow key
                    if (historyIndex < (int)history.size() - 1)
                    {
                        historyIndex++;
                        input = history[historyIndex];
                    }
                    else
                    {
                        historyIndex = history.size();
                        input.clear();
                    }
                    std::cout << "\33[2K\r";
                    std::cout.flush();
                    displayPrompt();
                    std::cout << input;
                    std::cout.flush();
                }
            }
        }
        // Normal characters
        else
        {
            input.push_back(c);
            std::cout << c;
            std::cout.flush();
        }
    }
}

std::string Shell::getPath()
{
    char cwd[PATH_MAX];
    getcwd(cwd, sizeof(cwd));
    return std::string(cwd);
}

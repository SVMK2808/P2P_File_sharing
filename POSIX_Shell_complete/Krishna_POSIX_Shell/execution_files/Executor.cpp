#include "Executor.h"
#include "Shell.h"
#include "RedirectionHandler.h"
#include <unistd.h>
#include <sys/wait.h>
#include <iostream>
#include <cstring>
#include <vector>
#include <limits.h>
#include <dirent.h>

namespace Executor
{

    void executepwd()
    {
        char cwd[PATH_MAX];
        if (getcwd(cwd, sizeof(cwd)) != nullptr)
        {
            std::cout << cwd << std::endl;
        }
        else
        {
            perror("pwd");
        }
    }

    void executeCd(const std::vector<std::string> &tokens)
    {
        std::string targetDir;

        if (tokens.size() == 1 || tokens[1] == "~")
        {
            const char *home = getenv("HOME");
            if (home != nullptr)
            {
                targetDir = home;
            }
            else
            {
                std::cerr << "cd : HOME not found";
                return;
            }
        }
        else
        {
            targetDir = tokens[1];
        }

        if (chdir(targetDir.c_str()) != 0)
        {
            perror("cd");
        }
    }
    void executeCommand(const std::vector<std::string> &tokens, bool isBackground)
    {
        if (tokens.empty())
            return;

        pid_t pid = fork();

        if (pid < 0)
        {
            std::cerr << "Error: fork failed\n";
            return;
        }

        if (pid == 0)
        {
            // Child process
            // std ::cout << "Child\n";

            if (tokens[0] == "pwd")
            {
                Executor::executepwd();
                return;
            }

            if (tokens[0] == "cd")
            {
                Executor::executeCd(tokens);
                return;
            }

            std::vector<std::string> Tokens = tokens;

            RedirectionHandler::handleRedirection(Tokens);

            std::vector<char *> args;
            for (const auto &token : Tokens)
            {
                args.push_back(const_cast<char *>(token.c_str()));
            }

            args.push_back(nullptr);

            execvp(args[0], args.data());

            std::cerr << "Error : Command not found" << tokens[0] << "\n";
            exit(EXIT_FAILURE);
        }
        else
        {
            // Parent process
            // std ::cout << "Parent\n";

            if (isBackground)
            {
                // std::cout << "a\n";
                std::cout << "Started background process with PID: " << pid << "\n";
            }
            else
            {
                int status;
                // std::cout << "b\n";
                Shell s;
                s.setFg_pid(pid);
                waitpid(pid, &status, 0); // wait for child to finish
            }
        }
    }
}
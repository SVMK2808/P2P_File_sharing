#include "Pipeliner.h"
#include "RedirectionHandler.h"
#include <unistd.h>
#include <sys/wait.h>
#include <vector>
#include <iostream>
#include <sstream>
#include <cstring>
#include "../header_files/Parser.h"
#include "../header_files/Shell.h"

Shell s;
namespace Pipeliner
{
    void executePipelineCommands(const std::vector<std::string> &commands, bool isBackground)
    {
        int numCommands = commands.size();
        int prevRead = -1;
        pid_t firstPid = -1;
        pid_t pgid = -1; // to hold the group leader PID
        std::vector<pid_t> pids;

        for (int i = 0; i < numCommands; i++)
        {
            int pipefd[2];
            if (i != numCommands - 1)
            {
                // Not the last command
                if (pipe(pipefd) < 0)
                {
                    perror("pipe");
                    return;
                }
            }

            pid_t pid = fork();

            if (pid < 0)
            {
                perror("fork");
                return;
            }

            if (pid == 0)
            {
                // Child process
                if (pgid == -1)
                {
                    pgid = getpid();
                }
                setpgid(0, pgid);

                // If not the first command, redirect stdin
                if (prevRead != -1)
                {
                    dup2(prevRead, STDIN_FILENO);
                    close(prevRead);
                }

                // If not the last command, redirect the stdout
                if (i != numCommands - 1)
                {

                    dup2(pipefd[1], STDOUT_FILENO);
                    close(pipefd[0]);
                    close(pipefd[1]);
                }

                // Tokenize the command
                std::vector<std::string> tokens = Parser::tokenizeCommand(commands[i]);
                RedirectionHandler::handleRedirection(tokens);
                std::vector<char *> args;
                for (const auto &token : tokens)
                {
                    args.push_back(const_cast<char *>(token.c_str()));
                }

                args.push_back(nullptr);

                execvp(args[0], args.data());
                perror("execvp");
                exit(EXIT_FAILURE);
            }
            else
            {
                // Parent Process
                if (pgid == -1)
                    pgid = pid;
                setpgid(pid, pgid);

                pids.push_back(pid);

                if (i == 0)
                {
                    firstPid = pid;
                    if (!isBackground)
                    {
                        tcsetpgrp(STDIN_FILENO, pgid);
                        s.setFg_pid(pgid);
                    }
                }
                if (prevRead != -1)
                    close(prevRead);
                if (i != numCommands - 1)
                {
                    close(pipefd[1]);     // Closing the write end
                    prevRead = pipefd[0]; // Save read end for the next command
                }
            }
        }

        if (isBackground)
        {
            std::cout << "Started background pipeline with PID : " << firstPid << "\n";
            std::cout.flush();
        }
        else
        {
            int status;
            for (auto pid : pids)
            {
                waitpid(pid, &status, WUNTRACED);
            }
            tcsetpgrp(STDIN_FILENO, getpgrp());
            s.setFg_pid(-1);
            s.renderPrompt();
            std::cout.flush();
        }
    }
}
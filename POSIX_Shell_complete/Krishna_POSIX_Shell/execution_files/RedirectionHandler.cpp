#include "RedirectionHandler.h"
#include <fcntl.h>
#include <unistd.h>
#include <iostream>

namespace RedirectionHandler
{
    void handleRedirection(std::vector<std::string> &tokens)
    {
        for (auto it = tokens.begin(); it != tokens.end();)
        {
            if (*it == "<" && (it + 1) != tokens.end())
            {
                int fd = open((it + 1)->c_str(), O_RDONLY);

                if (fd < 0)
                {
                    perror("Input file error");
                    return;
                }

                dup2(fd, STDIN_FILENO);
                close(fd);
                it = tokens.erase(it, it + 2);
            }
            else if (*it == ">" && (it + 1) != tokens.end())
            {
                int fd = open((it + 1)->c_str(), O_WRONLY | O_CREAT | O_TRUNC, 0644);
                if (fd < 0)
                {
                    perror("Output file error");
                    exit(EXIT_FAILURE);
                }
                dup2(fd, STDOUT_FILENO);
                close(fd);
                it = tokens.erase(it, it + 2);
            }
            else if (*it == ">>" && (it + 1) != tokens.end())
            {
                int fd = open((it + 1)->c_str(), O_WRONLY | O_CREAT | O_APPEND, 0644);
                if (fd < 0)
                {
                    perror("Append file error");
                    exit(EXIT_FAILURE);
                }

                dup2(fd, STDOUT_FILENO);
                close(fd);
                it = tokens.erase(it, it + 2);
            }
            else
            {
                ++it;
            }
        }
    }
}
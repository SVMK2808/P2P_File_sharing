#pragma once
#include <string>
#include <vector>

namespace Executor
{
    // This executes a command (foreground or background depending on the value of isBackground)
    void executeCommand(const std::vector<std::string> &tokens, bool isBackground);
    // Function to execute pwd
    void executepwd();
    // Function to execute cd
    void executeCd(std::vector<std::string> &tokens);
    // Function to handle ls command
    void run_ls(const std ::string &path, bool showAll);
}
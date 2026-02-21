#include "historyCommands.h"
#include <iostream>
#include <fstream>

static std::vector<std::string> commandHistory;
void saveToHistory(const std::string &cmd)
{
    if (!cmd.empty())
    {
        commandHistory.push_back(cmd);
        if (commandHistory.size() > 20)
        {
            commandHistory.erase(commandHistory.begin());
        }
    }
}

void handleHistoryCommand()
{
    int index = 1;
    for (const auto &cmd : commandHistory)
    {
        std::cout << index++ << " " << cmd << "\n";
    }
}

void saveToHistory()
{
    std::ofstream file(".my_shell_history");
    for (const auto &cmd : commandHistory)
    {
        file << cmd << "\n";
    }
}

void loadHistory()
{
    std::ifstream file(".my_shell_history");
    std::string line;
    while (std::getline(file, line))
    {
        if (!line.empty())
        {
            commandHistory.push_back(line);
        }
    }
}
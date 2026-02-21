#include "../header_files/Autocompleter.h"
#include <dirent.h>
#include <iostream>
#include <algorithm>
#include <unordered_set>
#include <sstream>
#include <sys/stat.h>

std::string Autocompleter::getLastToken(const std::string &input)
{
    size_t pos = input.find_last_of(" \t");
    return (pos == std::string::npos) ? input : input.substr(pos + 1);
}

std::vector<std::string> Autocompleter::getMatches(const std::string &prefix, bool isFirstToken)
{

    std::vector<std::string> matches;

    if (prefix.find('/') != std::string::npos || !isFirstToken)
    {
        std::string dirPath = ".";
        std::string filePrefix = prefix;

        size_t lastSlash = prefix.find_last_of('/');
        if (lastSlash != std::string::npos)
        {
            dirPath = prefix.substr(0, lastSlash);
            filePrefix = prefix.substr(lastSlash + 1);
        }

        DIR *dir = opendir(dirPath.c_str());
        if (!dir)
            return matches;

        struct dirent *entry;
        while ((entry = readdir(dir)) != nullptr)
        {
            std::string name(entry->d_name);
            if (name.find(filePrefix) == 0)
            {
                if (dirPath == ".")
                {
                    matches.push_back(name);
                }
                else
                {
                    matches.push_back(dirPath + '/' + name);
                }
            }
        }

        closedir(dir);
    }
    else
    {
        std::vector<std::string> paths = {"/bin", "/usr/bin", "/usr/local/bin"};

        for (const auto &path : paths)
        {
            DIR *dir = opendir(path.c_str());
            if (!dir)
                continue;

            struct dirent *entry;
            while ((entry = readdir(dir)))
            {
                std::string name(entry->d_name);
                if (name.rfind(prefix, 0) == 0)
                {
                    matches.push_back(name);
                }
            }
            closedir(dir);
        }
        std::vector<std::string> builtins = {"cd", "pwd", "echo", "pinfo"};
        for (const auto &cmd : builtins)
        {
            if (cmd.rfind(prefix, 0) == 0)
            {
                matches.push_back(cmd);
            }
        }
    }

    std::sort(matches.begin(), matches.end());
    return matches;
}

std::vector<std::string> Autocompleter::getCommandMatches(const std::string &prefix)
{
    std::vector<std::string> matches;
    const char *pathEnv = getenv("PATH");
    if (!pathEnv)
        return matches;

    std::stringstream ss(pathEnv);
    std::string dir;

    std::unordered_set<std::string> uniqueCommands;

    while (getline(ss, dir, ':'))
    {
        DIR *dp = opendir(dir.c_str());
        if (!dp)
            continue;

        struct dirent *entry;
        while ((entry = readdir(dp)) != NULL)
        {
            std::string fileName = entry->d_name;
            std::string fullPath = dir + "/" + fileName;

            struct stat sb;
            if (stat(fullPath.c_str(), &sb) == 0 && sb.st_mode & S_IXUSR)
            {
                if (fileName.rfind(prefix, 0) == 0 && uniqueCommands.count(fileName) == 0)
                {
                    matches.push_back(fileName);
                    uniqueCommands.insert(fileName);
                }
            }
        }
        closedir(dp);
    }
    return matches;
}

std::string Autocompleter::handleTabCompletion(const std::string &input, std::function<void()> displayPrompt)
{
    std::string lastWord = getLastToken(input);

    std::vector<std::string> matches;

    bool isFirstToken = input.find(' ') == std::string::npos;

    if (isFirstToken)
    {
        // Command autocompletion
        matches = getCommandMatches(lastWord);
    }

    else if (lastWord.find('/') != std::string::npos)
    {
        // complete path autocomplete
        matches = getMatches(lastWord, false);
    }
    else
    {
        // Simple file autocomplete
        matches = getMatches(lastWord, false);
    }

    std::string updatedInput = input;

    if (matches.empty())
    {
        // no match
        return updatedInput;
    }

    else if (matches.size() == 1)
    {
        size_t pos = updatedInput.rfind(lastWord);
        if (pos != std::string::npos)
        {
            updatedInput.erase(pos, lastWord.length());
            updatedInput += matches[0];

            std::cout << "\r";
            displayPrompt();
            std::cout << updatedInput;
            std::cout.flush();
        }
    }
    else
    {
        std::cout << "\n";
        for (const auto &match : matches)
        {
            std::cout << match << " ";
        }
        std::cout << "\n";

        displayPrompt();
        std::cout << input;
        std::cout.flush();
    }

    return updatedInput;
}

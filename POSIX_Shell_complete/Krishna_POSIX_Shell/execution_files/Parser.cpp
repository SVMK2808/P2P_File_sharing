#include "Parser.h"
#include <sstream>
#include <algorithm>
#include <cctype>

namespace Parser
{
    std::string trim(const std::string &str)
    {
        const auto begin = str.find_first_not_of(" \t\r\n");
        if (begin == std::string::npos)
            return "";
        const auto end = str.find_last_not_of(" \t\r\n");
        return str.substr(begin, end - begin + 1);
    }

    std::vector<std::string> SemiColonSplit(const std::string &input)
    {
        std::vector<std::string> commands;
        std::stringstream ss(input);
        std::string command;

        while (std::getline(ss, command, ';'))
        {
            std::string trimmed = trim(command);
            if (!trimmed.empty())
                commands.push_back(trimmed);
        }

        return commands;
    }

    std::vector<std::string> PipeOprSplit(const std::string &input)
    {
        std::vector<std::string> parts;
        std::stringstream ss(input);
        std::string part;
        while (std::getline(ss, part, '|'))
        {
            std::string trimmed = trim(part);
            if (!trimmed.empty())
                parts.push_back(trimmed);
        }
        return parts;
    }

    std::vector<std::string> tokenizeCommand(const std::string &input)
    {
        // std::vector<std::string> tokens;
        // std::istringstream iss(input);
        // std::string token;
        // while (iss >> token)
        // {
        //     tokens.push_back(token);
        // }
        // return tokens;
        return tokenizeRespectingQuotes(input);
    }

    std::vector<std::string> tokenizeRespectingQuotes(const std::string &input)
    {
        std::vector<std::string> tokens;
        std::string token;
        bool in_single_quote = false, in_double_quotes = false;
        std::string current;

        for (size_t i = 0; i < input.length(); i++)
        {
            char c = input[i];
            if (c == '"' && !in_single_quote)
            {
                in_double_quotes = !in_double_quotes;
                continue;
            }
            else if (c == '\'' && !in_double_quotes)
            {
                in_single_quote = !in_single_quote;
                continue;
            }

            if (std::isspace(c) && !in_single_quote && !in_double_quotes)
            {
                if (!current.empty())
                {
                    tokens.push_back(current);
                    current.clear();
                }
            }
            else
            {
                current += c;
            }
        }
        if (!current.empty())
            tokens.push_back(current);

        return tokens;
    }
}
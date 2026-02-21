#include <vector>
#include <string>
#include "Echo.h"
#include <iostream>
#include <regex>

void echoCommand(const std::vector<std::string> &tokens)
{
    for (size_t i = 1; i < tokens.size(); i++)
    {
        std::string token = tokens[i];
        if (!token.empty() && token.front() == '"' && token.back() == '"')
        {
            token = token.substr(1, token.length() - 2);

            std::regex varRegex(R"(\$[A-Za-z_][A-Za-z0-9_]*)");
            std::smatch match;

            std::string result;
            std::string::const_iterator searchStart(token.cbegin());

            while (std::regex_search(searchStart, token.cend(), match, varRegex))
            {
                result.append(match.prefix());
                std::string varName = match.str().substr(1);
                const char *value = getenv(varName.c_str());
                result.append(value ? value : "");
                searchStart = match.suffix().first;
            }

            result.append(searchStart, token.cend());
            std::cout << result;
        }
        else if (!token.empty() && token.front() == '\'' && token.back() == '\'')
        {
            std::cout << token.substr(1, token.length() - 2);
        }

        else if (!token.empty() && token[0] == '$')
        {
            std::string varName = token.substr(1);
            const char *value = getenv(varName.c_str());
            if (value != nullptr)
            {
                std::cout << value;
            }
            else
            {
                std::cout << "";
            }
        }
        else
        {
            std::cout << token;
        }

        if (i != tokens.size() - 1)
            std::cout << " ";
    }
    std::cout << "\n";
}
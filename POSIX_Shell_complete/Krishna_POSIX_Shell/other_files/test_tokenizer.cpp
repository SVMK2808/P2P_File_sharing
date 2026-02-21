#include <iostream>
#include <vector>
#include <string>
#include <cassert>

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
} // your tokenizer

extern std::vector<std::string> tokenizeCommand(const std::string &input)
{
    return tokenizeRespectingQuotes(input);
}

void runTests()
{
    std::vector<std::pair<std::string, std::vector<std::string>>> testCases = {
        {"echo Hello World", {"echo", "Hello", "World"}},
        {"cd /home/user/Documents", {"cd", "/home/user/Documents"}},
        {"echo \"Kanha is kind\"", {"echo", "Kanha is kind"}},
        {"echo This is \"one token\" and this is another", {"echo", "This", "is", "one token", "and", "this", "is", "another"}},
        {"echo 'Hare Krishna' and 'Radha Rani'", {"echo", "Hare Krishna", "and", "Radha Rani"}},
        {"echo \"Radha\" 'Krishna' Love", {"echo", "Radha", "Krishna", "Love"}},
        {"echo \"Radha's flute\"", {"echo", "Radha's flute"}},
        {"   echo    \"Trim   spaces\"     ", {"echo", "Trim   spaces"}},
        {"echo \"unclosed quote", {"echo", "unclosed quote"}}, // optional edge case
        {"echo \"quote 'inside' quote\"", {"echo", "quote 'inside' quote"}},
        {"ls execution_files | grep .cpp", {"ls", "execution_files", "|", "grep", ".cpp"}},
        {"cat a.txt", {"cat", "a.txt"}},
        {"cat < a.txt", {"cat", "<", "a.txt"}},
        {"cat Hello << a.txt", {"cat", "Hello", "<<", "a.txt"}},
        {"echo $HOME", {"echo", "$HOME"}}

    };

    int passed = 0;

    for (size_t i = 0; i < testCases.size(); ++i)
    {
        auto output = tokenizeCommand(testCases[i].first);
        if (output == testCases[i].second)
        {
            std::cout << "✅ Test " << i + 1 << " passed\n";
            passed++;
        }
        else
        {
            std::cout << "❌ Test " << i + 1 << " failed\n";
            std::cout << "Input: " << testCases[i].first << "\n";
            std::cout << "Expected: ";
            for (const auto &token : testCases[i].second)
                std::cout << "[" << token << "] ";
            std::cout << "\nGot: ";
            for (const auto &token : output)
                std::cout << "[" << token << "] ";
            std::cout << "\n";
        }
    }

    std::cout << "\n🎉 Passed " << passed << "/" << testCases.size() << " tests.\n";
}

int main()
{
    runTests();
    return 0;
}

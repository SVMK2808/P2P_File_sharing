#ifndef AUTOCOMPLETER_H
#define AUTOCOMPLETER_H

#include <string>
#include <vector>
#include <functional>

class Autocompleter
{
public:
    // Returns updated input string after autocomplete
    std::string handleTabCompletion(const std::string &input, std::function<void()> displayPrompt);

private:
    static std::string getLastToken(const std::string &input);
    static std::vector<std::string> getMatches(const std::string &prefix, bool isFirstToken);
    static std::vector<std::string> getCommandMatches(const std::string &prefix);
};

#endif
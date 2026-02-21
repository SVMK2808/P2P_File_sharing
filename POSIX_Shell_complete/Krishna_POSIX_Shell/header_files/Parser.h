#pragma once
#include <vector>
#include <string>

namespace Parser
{
    // Split by semicolon to separate the commands for execution
    std::vector<std::string> SemiColonSplit(const std::string &input);

    // Split by pipe '|' to implement the pipeline functionality
    std::vector<std::string> PipeOprSplit(const std::string &input);

    // Tokenise a single command to execute it
    std::vector<std::string> tokenizeCommand(const std::string &command);

    // Helper to trim whitespaces

    std::string trim(const std::string &str);
    std::vector<std::string> tokenizeRespectingQuotes(const std::string &input);
}
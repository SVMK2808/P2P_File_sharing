#pragma once
#include <string>
#include <vector>

namespace Pipeliner
{
    void executePipelineCommands(const std::vector<std::string> &commands, bool isBackground);
}
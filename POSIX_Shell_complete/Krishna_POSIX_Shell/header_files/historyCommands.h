#ifndef HISTORY_COMMAND_H
#define HISTORY_COMMAND_H

#include <vector>
#include <string>

void handleHistoryCommand();
void saveToHistory(const std::string &cmd);
void loadHistory();
void saveHistory();
#endif
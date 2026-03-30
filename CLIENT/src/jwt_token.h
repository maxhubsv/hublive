#pragma once

#include "config.h"
#include <string>

// Generates a HubLive-compatible JWT access token.
std::string GenerateAccessToken(const AppConfig& config);

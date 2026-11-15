#!/bin/bash
exec bash "$(cd "$(dirname "$0")" && pwd)/upgrade-rehearsal.sh" first "$@"

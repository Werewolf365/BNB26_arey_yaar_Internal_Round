# Dockerized Foundry wrapper (Windows PowerShell).
# The ghcr.io/foundry-rs/foundry image uses entrypoint `/bin/sh -c`,
# so tools MUST be selected via --entrypoint (else flags are swallowed
# and bare `forge` help is printed). Usage:
#   ./scripts/forge-docker.ps1 test -vvv
#   ./scripts/forge-docker.ps1 build
param([Parameter(ValueFromRemainingArguments = $true)][string[]]$ForgeArgs)
$repo = "C:\Users\Vicky\Desktop\QUORUM_BitNBuild\contracts"
$img = "ghcr.io/foundry-rs/foundry:latest"
& docker run --rm --entrypoint forge -v "${repo}:/project" -w /project $img @ForgeArgs
exit $LASTEXITCODE

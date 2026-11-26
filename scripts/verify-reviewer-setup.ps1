# Run after the two README setup commands. This never resets or seeds a database.
$ErrorActionPreference = 'Stop'

function Assert-DockerAvailable {
    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
        throw 'Docker is not installed. Install Docker with Compose, then run the README setup commands.'
    }
    docker info --format '{{.ServerVersion}}'
    if ($LASTEXITCODE -ne 0) { throw 'Docker is installed, but its daemon is unavailable.' }
}

function Assert-ComposeConfiguration {
    docker compose config --quiet
    if ($LASTEXITCODE -ne 0) { throw 'Compose configuration validation failed.' }
    docker compose ps
    if ($LASTEXITCODE -ne 0) { throw 'Could not inspect Compose services.' }
}

function Assert-FrontendAndAPI {
    $page = Invoke-WebRequest 'http://localhost:3000/' -UseBasicParsing -TimeoutSec 15
    if ($page.StatusCode -ne 200 -or $page.Content -notmatch '<div id="root">') {
        throw 'The frontend did not serve the application.'
    }
    # Read through Nginx so this also checks the same-origin API proxy.
    $areas = Invoke-RestMethod 'http://localhost:3000/api/areas' -TimeoutSec 15
    if (-not $areas) { throw 'The areas endpoint returned an empty response.' }
    # A weather outage can legitimately return 503. Print the response for review;
    # transport failures and all other unexpected HTTP errors still fail the check.
    try {
        $response = Invoke-WebRequest 'http://localhost:3000/api/recommendations' -UseBasicParsing -TimeoutSec 15
        $response.Content
    } catch {
        if ($null -eq $_.Exception.Response -or [int]$_.Exception.Response.StatusCode -ne 503) { throw }
        Write-Warning 'Recommendation unavailable (HTTP 503). Inspect API logs and weather access before submission.'
    }
}

Push-Location (Split-Path $PSScriptRoot -Parent)
try {
    Assert-DockerAvailable
    Assert-ComposeConfiguration
    Assert-FrontendAndAPI
    Write-Host 'Frontend and API proxy checks passed. Review the recommendation response and any warning above.'
} finally {
    Pop-Location
}

param(
    [string]$Lore,
    [string]$WorkDir,
    [string]$ClientKey,
    [string]$MCPAPIKey,
    [int]$TimeoutSeconds = 10,
    [switch]$KeepTemp
)

$ErrorActionPreference = "Stop"

$RepoRoot = Split-Path -Parent $PSScriptRoot
$Go = Join-Path $RepoRoot ".tools\go\bin\go.exe"
$TempRoot = $null
$Process = $null

if (-not (Test-Path $Go)) {
    $GoCommand = Get-Command go.exe -ErrorAction SilentlyContinue
    if ($null -eq $GoCommand) {
        throw "Go toolchain not found at $Go and go.exe is not on PATH"
    }
    $Go = $GoCommand.Source
}

function Invoke-Native {
    param(
        [Parameter(Mandatory = $true)]
        [string]$FilePath,
        [Parameter(Mandatory = $true)]
        [string[]]$Arguments
    )

    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$FilePath $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
}

function New-SmokeTempRoot {
    $root = Join-Path ([System.IO.Path]::GetTempPath()) ("lore-mcp-smoke-" + [System.Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $root | Out-Null
    return $root
}

function Remove-SmokeTempRoot {
    param([string]$Path)

    if ([string]::IsNullOrWhiteSpace($Path) -or -not (Test-Path -LiteralPath $Path)) {
        return
    }
    $resolvedPath = (Resolve-Path -LiteralPath $Path).Path
    $resolvedTemp = (Resolve-Path -LiteralPath ([System.IO.Path]::GetTempPath())).Path.TrimEnd('\')
    if (-not $resolvedPath.StartsWith($resolvedTemp, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "refusing to remove temp root outside system temp: $resolvedPath"
    }
    Remove-Item -LiteralPath $resolvedPath -Recurse -Force
}

function Quote-ProcessArgument {
    param([string]$Value)

    return '"' + ($Value -replace '"', '\"') + '"'
}

function Write-MCPRequest {
    param(
        [Parameter(Mandatory = $true)]
        [System.Diagnostics.Process]$Process,
        [Parameter(Mandatory = $true)]
        [int]$ID,
        [Parameter(Mandatory = $true)]
        [string]$Method,
        [Parameter(Mandatory = $true)]
        [object]$Params
    )

    $payload = [ordered]@{
        jsonrpc = "2.0"
        id = $ID
        method = $Method
        params = $Params
    } | ConvertTo-Json -Depth 20 -Compress
    $payloadBytes = [System.Text.Encoding]::UTF8.GetBytes($payload)
    $headerBytes = [System.Text.Encoding]::ASCII.GetBytes("Content-Length: $($payloadBytes.Length)`r`n`r`n")

    $stream = $Process.StandardInput.BaseStream
    $stream.Write($headerBytes, 0, $headerBytes.Length)
    $stream.Write($payloadBytes, 0, $payloadBytes.Length)
    $stream.Flush()
}

function Read-ExactBytes {
    param(
        [Parameter(Mandatory = $true)]
        [System.IO.Stream]$Stream,
        [Parameter(Mandatory = $true)]
        [int]$Count,
        [Parameter(Mandatory = $true)]
        [datetime]$Deadline
    )

    $buffer = New-Object byte[] $Count
    $offset = 0
    while ($offset -lt $Count) {
        $remaining = [int]([Math]::Max(1, ($Deadline - (Get-Date)).TotalMilliseconds))
        if ((Get-Date) -gt $Deadline) {
            throw "timed out while reading MCP response"
        }
        $task = $Stream.ReadAsync($buffer, $offset, $Count - $offset)
        if (-not $task.Wait($remaining)) {
            throw "timed out while reading MCP response"
        }
        $read = $task.Result
        if ($read -le 0) {
            throw "MCP server closed stdout"
        }
        $offset += $read
    }
    return $buffer
}

function Read-MCPResponse {
    param(
        [Parameter(Mandatory = $true)]
        [System.Diagnostics.Process]$Process,
        [Parameter(Mandatory = $true)]
        [int]$TimeoutSeconds
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $stream = $Process.StandardOutput.BaseStream
    $header = New-Object System.Collections.Generic.List[byte]
    while ($true) {
        $next = Read-ExactBytes -Stream $stream -Count 1 -Deadline $deadline
        $header.Add($next[0])
        $count = $header.Count
        if ($count -ge 4 -and $header[$count - 4] -eq 13 -and $header[$count - 3] -eq 10 -and $header[$count - 2] -eq 13 -and $header[$count - 1] -eq 10) {
            break
        }
    }

    $headerText = [System.Text.Encoding]::ASCII.GetString($header.ToArray())
    $contentLength = $null
    foreach ($line in ($headerText -split "`r`n")) {
        if ($line.ToLowerInvariant().StartsWith("content-length:")) {
            $contentLength = [int]($line.Substring("content-length:".Length).Trim())
        }
    }
    if ($null -eq $contentLength) {
        throw "MCP response missing Content-Length header: $headerText"
    }

    $body = Read-ExactBytes -Stream $stream -Count $contentLength -Deadline $deadline
    $json = [System.Text.Encoding]::UTF8.GetString($body)
    return $json | ConvertFrom-Json
}

function Assert-NoRPCError {
    param(
        [Parameter(Mandatory = $true)]
        [object]$Response,
        [Parameter(Mandatory = $true)]
        [string]$Step
    )

    if ($null -ne $Response.error) {
        throw "$Step returned JSON-RPC error $($Response.error.code): $($Response.error.message)"
    }
}

try {
    if ([string]::IsNullOrWhiteSpace($Lore) -or [string]::IsNullOrWhiteSpace($WorkDir)) {
        $TempRoot = New-SmokeTempRoot
    }

    if ([string]::IsNullOrWhiteSpace($Lore)) {
        if (-not (Test-Path -LiteralPath $Go)) {
            throw "Go toolchain not found at $Go"
        }
        $Lore = Join-Path $TempRoot "lore.exe"
        Push-Location $RepoRoot
        try {
            Invoke-Native -FilePath $Go -Arguments @("build", "-o", $Lore, "./cmd/lore")
        } finally {
            Pop-Location
        }
    }

    if ([string]::IsNullOrWhiteSpace($WorkDir)) {
        $WorkDir = Join-Path $TempRoot "workdir"
    }

    Invoke-Native -FilePath $Lore -Arguments @("bootstrap", $WorkDir)

    $startInfo = New-Object System.Diagnostics.ProcessStartInfo
    $startInfo.FileName = $Lore
    $startInfo.Arguments = (Quote-ProcessArgument "mcp") + " " + (Quote-ProcessArgument $WorkDir)
    $startInfo.UseShellExecute = $false
    $startInfo.RedirectStandardInput = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    foreach ($name in @("LORE_CLIENT_KEY", "OBSIDIAN_HARNESS_CLIENT_KEY", "LORE_MCP_API_KEY", "OBSIDIAN_HARNESS_MCP_API_KEY")) {
        $startInfo.EnvironmentVariables.Remove($name)
    }
    if (-not [string]::IsNullOrWhiteSpace($ClientKey)) {
        $startInfo.EnvironmentVariables["LORE_CLIENT_KEY"] = $ClientKey
    }
    if (-not [string]::IsNullOrWhiteSpace($MCPAPIKey)) {
        $startInfo.EnvironmentVariables["LORE_MCP_API_KEY"] = $MCPAPIKey
    }

    $Process = New-Object System.Diagnostics.Process
    $Process.StartInfo = $startInfo
    if (-not $Process.Start()) {
        throw "failed to start lore MCP process"
    }

    Write-MCPRequest -Process $Process -ID 1 -Method "initialize" -Params @{}
    $initialize = Read-MCPResponse -Process $Process -TimeoutSeconds $TimeoutSeconds
    Assert-NoRPCError -Response $initialize -Step "initialize"
    if ([string]::IsNullOrWhiteSpace($initialize.result.protocolVersion)) {
        throw "initialize response missing protocolVersion"
    }

    Write-MCPRequest -Process $Process -ID 2 -Method "ping" -Params @{}
    $ping = Read-MCPResponse -Process $Process -TimeoutSeconds $TimeoutSeconds
    Assert-NoRPCError -Response $ping -Step "ping"

    Write-MCPRequest -Process $Process -ID 3 -Method "tools/list" -Params @{}
    $toolsList = Read-MCPResponse -Process $Process -TimeoutSeconds $TimeoutSeconds
    Assert-NoRPCError -Response $toolsList -Step "tools/list"
    $toolNames = @($toolsList.result.tools | ForEach-Object { $_.name })
    foreach ($requiredTool in @("managed_status", "vault_resolve", "context_pack")) {
        if ($toolNames -notcontains $requiredTool) {
            throw "tools/list missing $requiredTool"
        }
    }

    Write-MCPRequest -Process $Process -ID 4 -Method "tools/call" -Params @{
        name = "managed_status"
        arguments = @{}
    }
    $status = Read-MCPResponse -Process $Process -TimeoutSeconds $TimeoutSeconds
    Assert-NoRPCError -Response $status -Step "tools/call managed_status"
    if ($status.result.isError) {
        throw "managed_status returned isError=true"
    }
    if ($status.result.structuredContent.ready -ne $true) {
        throw "managed_status ready = $($status.result.structuredContent.ready), want true"
    }

    Write-Host "MCP stdio smoke passed: $Lore mcp $WorkDir"
} finally {
    if ($null -ne $Process -and -not $Process.HasExited) {
        $Process.Kill()
        $Process.WaitForExit()
    }
    if (-not $KeepTemp -and $null -ne $TempRoot) {
        Remove-SmokeTempRoot -Path $TempRoot
    } elseif ($null -ne $TempRoot) {
        Write-Host "Kept temp root: $TempRoot"
    }
}

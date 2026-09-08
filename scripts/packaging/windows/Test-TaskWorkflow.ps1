[CmdletBinding()]
param(
    [string]$BaseUrl = 'https://127.0.0.1:8443'
)

$ErrorActionPreference = 'Stop'
$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession

function Invoke-JsonRequest {
    param(
        [Parameter(Mandatory = $true)] [ValidateSet('GET', 'POST', 'PUT', 'DELETE')] [string]$Method,
        [Parameter(Mandatory = $true)] [string]$Path,
        [object]$Body,
        [int]$ExpectedStatus = 200
    )

    $request = @{
        Uri = "$BaseUrl$Path"
        Method = $Method
        WebSession = $session
        SkipCertificateCheck = $true
        UseBasicParsing = $true
        ErrorAction = 'Stop'
    }
    if ($null -ne $Body) {
        $request.ContentType = 'application/json'
        $request.Body = ($Body | ConvertTo-Json -Compress)
    }
    $response = Invoke-WebRequest @request
    if ($response.StatusCode -ne $ExpectedStatus) {
        throw "$Method $Path returned HTTP $($response.StatusCode), expected $ExpectedStatus"
    }
    if ([string]::IsNullOrWhiteSpace($response.Content)) { return $null }
    return ($response.Content | ConvertFrom-Json)
}

$suffix = [Guid]::NewGuid().ToString('N')
$email = "packaged-$suffix@example.invalid"
$password = 'packaged-integration-password-123'

$registration = Invoke-JsonRequest -Method POST -Path '/auth/register' -Body @{ email = $email; password = $password } -ExpectedStatus 201
if ([int]$registration.user_id -le 0) { throw 'registration did not return a user ID' }

$backends = @(Invoke-JsonRequest -Method GET -Path '/backends')
if ($backends.Count -lt 1) { throw 'authenticated user has no default backend' }
$backendId = [int]$backends[0].id
if ($backendId -le 0) { throw 'default backend did not return a valid ID' }

$task = Invoke-JsonRequest -Method POST -Path '/caldav/tasks' -Body @{
    backend_id = $backendId
    title = 'Windows packaged workflow task'
    status = 'NEEDS-ACTION'
} -ExpectedStatus 201
$taskId = [int]$task.id
if ($taskId -le 0) { throw 'task creation did not return a valid ID' }

$updated = Invoke-JsonRequest -Method PUT -Path "/caldav/tasks/$taskId" -Body @{
    backend_id = $backendId
    title = 'Windows packaged workflow task updated'
    status = 'IN-PROCESS'
} -ExpectedStatus 200
if ($updated.title -ne 'Windows packaged workflow task updated') { throw 'task update response was not persisted' }

$read = Invoke-JsonRequest -Method GET -Path "/caldav/tasks/$taskId"
if ($read.title -ne 'Windows packaged workflow task updated') { throw 'updated task could not be read' }

$deleteResponse = Invoke-JsonRequest -Method DELETE -Path "/caldav/tasks/$taskId" -ExpectedStatus 204
$remaining = @(Invoke-JsonRequest -Method GET -Path "/caldav/tasks?backend_id=$backendId")
if ($remaining | Where-Object { [int]$_.id -eq $taskId }) { throw 'deleted task remained in the task list' }

$logoutRequest = @{
    Uri = "$BaseUrl/auth/logout"
    Method = 'POST'
    WebSession = $session
    SkipCertificateCheck = $true
    UseBasicParsing = $true
    ErrorAction = 'Stop'
}
$logout = Invoke-WebRequest @logoutRequest
if ($logout.StatusCode -ne 204) { throw "POST /auth/logout returned HTTP $($logout.StatusCode), expected 204" }

Write-Host 'Windows packaged authenticated task workflow passed.'

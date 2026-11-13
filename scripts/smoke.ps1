# End-to-end smoke test for the knot API.
#
# It needs a FRESH database: setup only succeeds while no account exists, and
# several checks depend on being able to promote other users. Point a throwaway
# instance at an empty file, for example:
#
#   $env:KNOT_DB = "$env:TEMP/knot-smoke.db"
#   Remove-Item $env:KNOT_DB -ErrorAction SilentlyContinue
#   knot serve
#   powershell -File scripts/smoke.ps1 -Base http://localhost:8080
#
# Every check prints PASS or FAIL and the script exits non-zero if anything
# failed, so it can gate a deploy.

param(
    [string]$Base = "http://localhost:8080"
)

$ErrorActionPreference = "Stop"
$script:failures = 0
$script:checks = 0

function Check {
    param([string]$Name, [scriptblock]$Block)
    $script:checks++
    try {
        $result = & $Block
        if ($result -eq $false) {
            throw "assertion returned false"
        }
        Write-Host "PASS  $Name" -ForegroundColor Green
    }
    catch {
        $script:failures++
        Write-Host "FAIL  $Name" -ForegroundColor Red
        if ($_.ErrorDetails.Message) {
            Write-Host "      $($_.ErrorDetails.Message)" -ForegroundColor DarkRed
        }
        else {
            Write-Host "      $($_.Exception.Message)" -ForegroundColor DarkRed
        }
    }
}

function New-Session {
    return New-Object Microsoft.PowerShell.Commands.WebRequestSession
}

function Call {
    param(
        [string]$Method,
        [string]$Path,
        $Body = $null,
        $Session = $null
    )
    $params = @{
        Method      = $Method
        Uri         = "$Base$Path"
        ErrorAction = 'Stop'
    }
    if ($Session) { $params.WebSession = $Session }
    if ($null -ne $Body) {
        $params.Body = ($Body | ConvertTo-Json -Depth 8)
        $params.ContentType = 'application/json'
    }
    return Invoke-RestMethod @params
}

$suffix = [guid]::NewGuid().ToString('N').Substring(0, 8)
$adminName = "smoke$suffix"
$modName = "mod$suffix"
$memberName = "member$suffix"
$password = 'correct-horse-battery'
$marker = "zebracorn$suffix"

$admin = New-Session
$mod = New-Session
$member = New-Session

Check "health endpoint responds" {
    $h = Call GET "/api/v1/health"
    $h.status -eq 'ok'
}

Check "meta reports site settings" {
    $m = Call GET "/api/v1/meta"
    $m.site_name.Length -gt 0
}

Check "fresh instance reports that it needs setup" {
    $s = Call GET "/api/v1/setup"
    $s.needs_setup -eq $true
}

Check "setup creates the administrator" {
    $r = Call POST "/api/v1/setup" @{
        site_name = "knot smoke"
        username  = $adminName
        email     = "$adminName@example.com"
        password  = $password
    } $admin
    $r.username -eq $adminName -and $r.role -eq 'admin'
}

Check "setup closes once an account exists" {
    try {
        Call POST "/api/v1/setup" @{
            username = "late$suffix"
            email    = "late$suffix@example.com"
            password = $password
        } (New-Session) | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 409
    }
}

Check "setup is no longer offered after setup" {
    $s = Call GET "/api/v1/setup"
    $s.needs_setup -eq $false
}

Check "session cookie is httpOnly" {
    $cookie = $admin.Cookies.GetCookies($Base)['knot_session']
    $cookie -ne $null -and $cookie.HttpOnly
}

Check "duplicate username is rejected" {
    try {
        Call POST "/api/v1/auth/register" @{ username = $adminName; password = $password } | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 409
    }
}

Check "wrong password is rejected" {
    try {
        $other = New-Session
        Call POST "/api/v1/auth/login" @{ login = $adminName; password = 'not-the-password' } $other | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 401
    }
}

Check "login succeeds with the right password" {
    $other = New-Session
    $me = Call POST "/api/v1/auth/login" @{ login = $adminName; password = $password } $other
    $me.username -eq $adminName
}

Check "anonymous cannot read /auth/me" {
    try {
        Call GET "/api/v1/auth/me" | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 401
    }
}

Check "boards are seeded" {
    $b = Call GET "/api/v1/boards"
    $b.boards.Count -ge 1
}

$threadId = $null
$firstPostId = $null

Check "create a thread" {
    $t = Call POST "/api/v1/boards/general/threads" @{ title = "Smoke test $suffix"; body = "Hello world $marker" } $admin
    $script:threadId = $t.thread.id
    $t.thread.title -eq "Smoke test $suffix"
}

Check "thread page returns the first post" {
    $page = Call GET "/api/v1/threads/$script:threadId" $null $admin
    $script:firstPostId = $page.posts[0].id
    $page.posts.Count -eq 1 -and $page.posts[0].body_html -like '*Hello world*'
}

Check "script in markdown is stripped" {
    $page = Call GET "/api/v1/threads/$script:threadId" $null $admin
    $page.posts[0].body_html -notlike '*<script>*'
}

Check "reply is appended" {
    $p = Call POST "/api/v1/threads/$script:threadId/posts" @{ body = "A reply mentioning nobody" } $admin
    $p.post.id -gt 0
}

Check "thread counters update" {
    $page = Call GET "/api/v1/threads/$script:threadId" $null $admin
    $page.thread.post_count -eq 2
}

Check "second user registers" {
    $r = Call POST "/api/v1/auth/register" @{ username = $modName; email = "$modName@example.com"; password = $password } $mod
    $r.username -eq $modName
}

Check "a plain member registers" {
    $r = Call POST "/api/v1/auth/register" @{ username = $memberName; email = "$memberName@example.com"; password = $password } $member
    $r.username -eq $memberName
}

Check "member cannot pin a thread" {
    try {
        Call POST "/api/v1/threads/$script:threadId/pin" $null $mod | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 403
    }
}

Check "admin promotes the second user to moderator" {
    $me = Call GET "/api/v1/auth/me" $null $mod
    $r = Call PATCH "/api/v1/admin/users/$($me.id)" @{ role = 'moderator' } $admin
    $r.role -eq 'moderator'
}

Check "moderator can pin" {
    $r = Call POST "/api/v1/threads/$script:threadId/pin" $null $mod
    $r.thread.pinned -eq $true
}

Check "reply notifies the thread author" {
    Call POST "/api/v1/threads/$script:threadId/posts" @{ body = "Reply from the moderator" } $mod | Out-Null
    $n = Call GET "/api/v1/notifications" $null $admin
    # @() is required: a filtered result of exactly one object has no .Count in
    # Windows PowerShell 5.1, so $null -ge 1 would silently read as false.
    @($n.notifications | Where-Object { $_.kind -eq 'reply' }).Count -ge 1
}

Check "mention notifies the mentioned user" {
    Call POST "/api/v1/threads/$script:threadId/posts" @{ body = "Pinging @$adminName about this" } $mod | Out-Null
    $n = Call GET "/api/v1/notifications" $null $admin
    @($n.notifications | Where-Object { $_.kind -eq 'mention' }).Count -ge 1
}

Check "voting updates the score" {
    $v = Call POST "/api/v1/posts/$script:firstPostId/vote" @{ value = 1 } $mod
    $v.score -eq 1 -and $v.my_vote -eq 1
}

Check "unvoting returns the score to zero" {
    $v = Call POST "/api/v1/posts/$script:firstPostId/vote" @{ value = 0 } $mod
    $v.score -eq 0
}

Check "full-text search finds the thread" {
    $s = Call GET "/api/v1/search?q=$marker" $null $admin
    $s.results.Count -ge 1
}

Check "search tolerates operator characters" {
    $s = Call GET "/api/v1/search?q=%22%28%2A" $null $admin
    $s.query.Length -gt 0
}

# A snippet quotes post text, and the index stores that text as plain text. Typed
# inside a code block the payload below reaches the index with its angle brackets
# intact, so the snippet is the one place post text becomes HTML in the browser.
Check "a search snippet is escaped, not rendered" {
    $needle = "sneaky$suffix"
    $body = '`<img src=x onerror=alert(1)> ' + $needle + '`'
    Call POST "/api/v1/threads/$script:threadId/posts" @{ body = $body } $admin | Out-Null

    $s = Call GET "/api/v1/search?q=$needle" $null $admin
    $hit = @($s.results | Where-Object { $_.snippet -like "*$needle*" })[0]
    if (-not $hit) { throw "the payload post was not indexed" }
    if ($hit.snippet -like '*<img*') { throw "snippet contains a live tag: $($hit.snippet)" }
    $hit.snippet -like '*&lt;img*'
}

Check "reporting a post works" {
    $r = Call POST "/api/v1/posts/$script:firstPostId/report" @{ reason = 'spam'; details = 'smoke test' } $mod
    $r.id -gt 0
}

Check "moderator sees the report" {
    $r = Call GET "/api/v1/admin/reports" $null $mod
    $r.reports.Count -ge 1
}

Check "moderator resolves the report" {
    $list = Call GET "/api/v1/admin/reports" $null $mod
    $id = $list.reports[0].id
    $r = Call POST "/api/v1/admin/reports/$id/resolve" @{ status = 'resolved' } $mod
    $r.ok -eq $true
}

Check "member cannot read admin settings" {
    try {
        Call GET "/api/v1/admin/settings" $null $mod | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 403
    }
}

Check "admin can read admin settings" {
    $r = Call GET "/api/v1/admin/settings" $null $admin
    $r.settings.site_name.Length -gt 0
}

Check "admin stats are returned" {
    $r = Call GET "/api/v1/admin/stats" $null $admin
    $r.users -ge 2 -and $r.threads -ge 1
}

Check "locked thread rejects replies from members" {
    Call POST "/api/v1/threads/$script:threadId/lock" $null $mod | Out-Null
    try {
        Call POST "/api/v1/threads/$script:threadId/posts" @{ body = "should not work" } $member | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 403
    }
}

Check "moderators can still reply to a locked thread" {
    # The lock is a rule for members; moderators keep posting so they can
    # explain why a thread was closed.
    $p = Call POST "/api/v1/threads/$script:threadId/posts" @{ body = "moderator bypass" } $mod
    $p.post.id -gt 0
}

Check "logout clears the session" {
    Call POST "/api/v1/auth/logout" $null $mod | Out-Null
    try {
        Call GET "/api/v1/auth/me" $null $mod | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 401
    }
}

Check "profile update persists" {
    $r = Call PATCH "/api/v1/me" @{ display_name = "Smoke Tester"; bio = "testing"; avatar_url = "" } $admin
    $r.display_name -eq 'Smoke Tester'
}

# A remote avatar makes every reader's browser call a third party, which reveals
# who is reading what, so it is opt-in.
Check "a remote avatar is refused" {
    try {
        Call PATCH "/api/v1/me" @{ avatar_url = "https://example.com/avatar.png" } $admin | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 400
    }
}

Check "an uploaded avatar is accepted" {
    $r = Call PATCH "/api/v1/me" @{ avatar_url = "/uploads/smoke.png" } $admin
    $r.avatar_url -eq "/uploads/smoke.png"
}

Check "an avatar can be cleared again" {
    $r = Call PATCH "/api/v1/me" @{ avatar_url = "" } $admin
    $r.avatar_url -eq ""
}

# ---- spam protection ----
#
# These run last because they change site settings. Every rule is checked from
# both sides: that it blocks what it should, and that it does not block what it
# should not.

$script:fresh = New-Session
$freshName = "fresh$suffix"

Check "a new account can register and post plain text" {
    Call POST "/api/v1/auth/register" @{ username = $freshName; password = $password } $script:fresh | Out-Null
    $t = Call POST "/api/v1/boards/general/threads" @{ title = "Plain thread $suffix"; body = "No links here." } $script:fresh
    $t.thread.pending -eq $false
}

Check "a new account cannot post a link" {
    try {
        Call POST "/api/v1/boards/general/threads" @{
            title = "Cheap deals"; body = "[click here](https://spam.example/offer)"
        } $script:fresh | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 403
    }
}

Check "a new account cannot smuggle a link in by editing" {
    $mine = Call POST "/api/v1/boards/general/threads" @{ title = "Edit test $suffix"; body = "clean" } $script:fresh
    $post = (Call GET "/api/v1/threads/$($mine.thread.id)" $null $script:fresh).posts[0]
    try {
        Call PATCH "/api/v1/posts/$($post.id)" @{ body = "now with https://spam.example" } $script:fresh | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 403
    }
}

Check "links to the forum itself are allowed" {
    # The forum's own host is never treated as off-site. This assumes the
    # instance was started with the development defaults, where the API and the
    # public address share the hostname.
    $t = Call POST "/api/v1/boards/general/threads" @{
        title = "Internal $suffix"; body = "see [thread]($Base/t/1)"
    } $script:fresh
    $t.thread.id -gt 0
}

Check "the registration honeypot is rejected" {
    try {
        Call POST "/api/v1/auth/register" @{
            username = "bot$suffix"; password = $password; website = "https://spam.example"
        } | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 400
    }
}

Check "an instant registration is rejected" {
    try {
        Call POST "/api/v1/auth/register" @{
            username = "fast$suffix"; password = $password; elapsed_ms = 5
        } | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 400
    }
}

Check "a blocked e-mail domain is refused" {
    Call PATCH "/api/v1/admin/settings" @{ settings = @{ spam_blocked_email_domains = "mailinator.com" } } $admin | Out-Null
    try {
        Call POST "/api/v1/auth/register" @{
            username = "burner$suffix"; email = "throwaway@mailinator.com"; password = $password
        } | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 400
    }
}

Check "the daily post limit applies to new accounts" {
    # $script:fresh has posted twice already.
    Call PATCH "/api/v1/admin/settings" @{ settings = @{ spam_new_posts_per_day = "2" } } $admin | Out-Null
    try {
        Call POST "/api/v1/boards/general/threads" @{
            title = "One too many $suffix"; body = "third post of the day"
        } $script:fresh | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 429
    }
}

Check "the daily limit does not apply to established accounts" {
    $t = Call POST "/api/v1/boards/general/threads" @{ title = "Admin post $suffix"; body = "unaffected" } $admin
    $t.thread.id -gt 0
}

$script:heldId = 0
$script:heldAuthor = New-Session

Check "a held thread is created unpublished" {
    Call PATCH "/api/v1/admin/settings" @{
        settings = @{ spam_approve_first_posts = "5"; spam_new_posts_per_day = "50" }
    } $admin | Out-Null

    Call POST "/api/v1/auth/register" @{ username = "held$suffix"; password = $password } $script:heldAuthor | Out-Null
    $t = Call POST "/api/v1/boards/general/threads" @{ title = "Held $suffix"; body = "waiting for review" } $script:heldAuthor
    $script:heldId = $t.thread.id
    $t.thread.pending -eq $true
}

Check "the author can see their own held thread" {
    $t = Call GET "/api/v1/threads/$script:heldId" $null $script:heldAuthor
    $t.thread.pending -eq $true -and @($t.posts).Count -eq 1
}

Check "nobody else can see a held thread" {
    try {
        Call GET "/api/v1/threads/$script:heldId" $null $member | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 404
    }
}

# The thread view hides held threads; the single-post view has to agree, or the
# queue can be read by walking ids.
Check "the opening post of a held thread is not readable by id" {
    $t = Call GET "/api/v1/threads/$script:heldId" $null $script:heldAuthor
    $script:heldPostId = @($t.posts)[0].id
    try {
        Call GET "/api/v1/posts/$script:heldPostId" $null $member | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 404
    }
}

Check "the author can still read their own held post" {
    $p = Call GET "/api/v1/posts/$script:heldPostId" $null $script:heldAuthor
    $p.post.id -eq $script:heldPostId
}

Check "a stranger cannot reply into a held thread" {
    try {
        Call POST "/api/v1/threads/$script:heldId/posts" @{ body = "sneaking into the queue" } $member | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 404
    }
}

Check "a held thread stays out of the board list" {
    $list = Call GET "/api/v1/boards/general/threads?per_page=100" $null $admin
    @($list.threads | Where-Object { $_.id -eq $script:heldId }).Count -eq 0
}

Check "held content is not in search results" {
    $s = Call GET "/api/v1/search?q=waiting" $null $admin
    @($s.results | Where-Object { $_.thread_id -eq $script:heldId }).Count -eq 0
}

Check "the review queue lists the held thread" {
    $q = Call GET "/api/v1/admin/pending" $null $admin
    @($q.threads | Where-Object { $_.id -eq $script:heldId }).Count -eq 1
}

Check "a member cannot read the review queue" {
    try {
        Call GET "/api/v1/admin/pending" $null $member | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 403
    }
}

Check "approving publishes the thread and its first post" {
    Call POST "/api/v1/admin/threads/$script:heldId/approve" $null $admin | Out-Null
    $t = Call GET "/api/v1/threads/$script:heldId" $null $member
    $t.thread.pending -eq $false -and @($t.posts).Count -eq 1
}

Check "an approved thread appears in the board list" {
    $list = Call GET "/api/v1/boards/general/threads?per_page=100" $null $admin
    @($list.threads | Where-Object { $_.id -eq $script:heldId }).Count -eq 1
}

Check "a held reply does not change the thread counters" {
    $before = (Call GET "/api/v1/threads/$script:heldId" $null $admin).thread.post_count
    $p = Call POST "/api/v1/threads/$script:heldId/posts" @{ body = "second held post" } $script:heldAuthor
    $after = (Call GET "/api/v1/threads/$script:heldId" $null $admin).thread.post_count
    $p.post.pending -eq $true -and $before -eq $after
}

Check "approving a reply publishes it and bumps the count" {
    $q = Call GET "/api/v1/admin/pending" $null $admin
    $pending = @($q.posts | Where-Object { $_.post.thread_id -eq $script:heldId })
    if ($pending.Count -ne 1) { throw "expected one held reply, found $($pending.Count)" }
    Call POST "/api/v1/admin/posts/$($pending[0].post.id)/approve" $null $admin | Out-Null
    (Call GET "/api/v1/threads/$script:heldId" $null $admin).thread.post_count -eq 2
}

Check "an older account earns the right to post links" {
    # $script:heldAuthor now has 2 published posts; drop the bar to 2 posts and
    # 0 days so the rule can be shown to lift.
    Call PATCH "/api/v1/admin/settings" @{
        settings = @{ spam_links_require_posts = "2"; spam_links_require_days = "0" }
    } $admin | Out-Null
    $t = Call POST "/api/v1/boards/general/threads" @{
        title = "Now allowed $suffix"; body = "see [example](https://example.com)"
    } $script:heldAuthor
    $t.thread.id -gt 0
}

Check "settings can be restored to the defaults" {
    $s = Call PATCH "/api/v1/admin/settings" @{
        settings = @{
            spam_links_require_posts = "3"
            spam_links_require_days = "2"
            spam_new_posts_per_day = "20"
            spam_approve_first_posts = "0"
            spam_blocked_email_domains = ""
        }
    } $admin
    $s.settings.spam_links_require_posts -eq "3" -and $s.settings.spam_approve_first_posts -eq "0"
}

Check "a nonsense spam threshold is refused" {
    try {
        Call PATCH "/api/v1/admin/settings" @{ settings = @{ spam_new_posts_per_day = "often" } } $admin | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 400
    }
}

# ---- e-mail ----
#
# There is no real mail server here, so delivery is expected to fail. What is
# checked is that failures are visible, that nothing blocks a request, and that
# the reset flow works end to end.

$script:resetEmail = "reset$suffix@example.com"
$script:resetUser = New-Session
$script:resetToken = ""
$script:mailId = 0

Check "password reset is unavailable without a mail server" {
    try {
        Call POST "/api/v1/auth/reset" @{ email = "nobody$suffix@example.com" } | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 501
    }
}

Check "an SMTP test reports the relay's own error" {
    Call PATCH "/api/v1/admin/settings" @{
        settings = @{ smtp_host = "127.0.0.1"; smtp_port = "1"; smtp_from = "forum@example.com" }
    } $admin | Out-Null

    try {
        Call POST "/api/v1/admin/mail/test" @{ to = "admin@example.com" } $admin | Out-Null
        $false
    }
    catch {
        $status = $_.Exception.Response.StatusCode.value__
        $body = $_.ErrorDetails.Message
        $status -eq 502 -and $body -like "*connect to 127.0.0.1:1*"
    }
}

Check "the SMTP password is never sent back to the browser" {
    Call PATCH "/api/v1/admin/settings" @{ settings = @{ smtp_pass = "not-a-real-password" } } $admin | Out-Null
    $s = Call GET "/api/v1/admin/settings" $null $admin
    ($null -eq $s.settings.smtp_pass) -and ($s.settings.smtp_password_set -eq "true")
}

Check "a reset link is queued rather than sent during the request" {
    Call POST "/api/v1/auth/register" @{
        username = "reset$suffix"; email = $script:resetEmail; password = $password
    } $script:resetUser | Out-Null

    $r = Call POST "/api/v1/auth/reset" @{ email = $script:resetEmail }
    $q = Call GET "/api/v1/admin/mail" $null $admin
    ($r.message -like "*reset link*") -and
        (@($q.items | Where-Object { $_.to -eq $script:resetEmail }).Count -ge 1)
}

Check "the queue listing redacts sign-in links" {
    $q = Call GET "/api/v1/admin/mail" $null $admin
    $item = @($q.items | Where-Object { $_.to -eq $script:resetEmail })[0]
    if (-not $item) { throw "nothing queued for $($script:resetEmail)" }
    $script:mailId = $item.id

    # The body is still there to diagnose a delivery failure, but the token that
    # makes the link work must not be: a screenshot of the outbox is not a
    # credential. Contains() rather than -like: wildcards read [redacted] as a
    # character class and would quietly match anything.
    $item.body.Contains("token=[redacted]") -and ($item.body -notmatch 'token=[A-Za-z0-9_\-]{20,}')
}

Check "a plain member cannot read a queued message" {
    try {
        Call GET "/api/v1/admin/mail/$($script:mailId)" $null $member | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 403
    }
}

Check "an admin can read a queued message deliberately" {
    $full = Call GET "/api/v1/admin/mail/$script:mailId" $null $admin
    $script:resetToken = if ($full.message.body -match 'token=([A-Za-z0-9_\-]+)') { $matches[1] } else { "" }
    $script:resetToken.Length -ge 20
}

Check "a failed delivery is recorded with its error" {
    # The worker dials 127.0.0.1:1 as soon as something is queued, but the attempt
    # happens on another goroutine, so give it a moment rather than guessing.
    $item = $null
    for ($i = 0; $i -lt 30; $i++) {
        $q = Call GET "/api/v1/admin/mail" $null $admin
        $item = @($q.items | Where-Object { $_.to -eq $script:resetEmail })[0]
        if ($item -and $item.attempts -ge 1) { break }
        Start-Sleep -Milliseconds 100
    }
    if (-not $item) { throw "nothing queued for $($script:resetEmail)" }

    ($item.attempts -ge 1) -and ($item.last_error -like "*127.0.0.1*")
}

Check "the reset token sets a new password" {
    Call POST "/api/v1/auth/reset/confirm" @{
        token = $script:resetToken; password = "a-brand-new-passphrase"
    } | Out-Null

    $fresh = New-Session
    $me = Call POST "/api/v1/auth/login" @{ login = "reset$suffix"; password = "a-brand-new-passphrase" } $fresh
    $me.username -eq "reset$suffix"
}

Check "a reset signs out the account's other sessions" {
    try {
        Call GET "/api/v1/auth/me" $null $script:resetUser | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 401
    }
}

Check "a reset token cannot be used twice" {
    try {
        Call POST "/api/v1/auth/reset/confirm" @{
            token = $script:resetToken; password = "yet-another-passphrase"
        } | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 404
    }
}

Check "an unknown address gets the same answer as a real one" {
    $r = Call POST "/api/v1/auth/reset" @{ email = "ghost$suffix@example.com" }
    $r.message -like "*reset link*"
}

Check "mail settings are restored to unconfigured" {
    $s = Call PATCH "/api/v1/admin/settings" @{ settings = @{ smtp_host = "" } } $admin
    $s.settings.smtp_host -eq ""
}

# ---- backups ----

$script:backupName = $null

Check "an admin can take a backup" {
    $r = Call POST "/api/v1/admin/backups" $null $admin
    $script:backupName = $r.archive.name
    $r.archive.name -like 'knot-*.zip' -and $r.archive.size -gt 0
}

Check "the backup is listed with its retention setting" {
    $b = Call GET "/api/v1/admin/backups" $null $admin
    (@($b.archives | Where-Object { $_.name -eq $script:backupName }).Count -eq 1) -and
    ($b.keep -ge 1)
}

Check "the backup downloads as a zip" {
    $path = Join-Path $env:TEMP "knot-smoke-$suffix.zip"
    try {
        $response = Invoke-WebRequest -Method GET -Uri "$Base/api/v1/admin/backups/$script:backupName" `
            -WebSession $admin -UseBasicParsing -OutFile $path -PassThru -ErrorAction Stop
        $bytes = [System.IO.File]::ReadAllBytes($path)
        ($response.Headers['Content-Type'] -eq 'application/zip') -and
        ($bytes.Length -gt 1000) -and
        ($bytes[0] -eq 0x50) -and
        ($bytes[1] -eq 0x4B)
    }
    finally {
        Remove-Item $path -ErrorAction SilentlyContinue
    }
}

Check "a made-up backup name is refused" {
    try {
        Call GET "/api/v1/admin/backups/knot-19700101-000000.zip" $null $admin | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 404
    }
}

Check "a backup name that is not an archive is refused" {
    try {
        Call GET "/api/v1/admin/backups/passwd.txt" $null $admin | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 400
    }
}

Check "a plain member cannot list backups" {
    try {
        Call GET "/api/v1/admin/backups" $null $member | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 403
    }
}

# ---- sitemap ----

Check "the sitemap lists boards and threads" {
    $s = Call GET "/api/v1/sitemap"
    (@($s.boards).Count -ge 1) -and
    (@($s.threads | Where-Object { $_.id -eq $script:threadId }).Count -eq 1)
}

# The CSRF probe goes last: in Windows PowerShell 5.1 a -Headers value is kept on
# the WebSession, so every later request from that session would carry the hostile
# Origin and fail with bad_origin for reasons that have nothing to do with CSRF.
Check "cross-site POST is rejected" {
    try {
        Invoke-RestMethod -Method POST -Uri "$Base/api/v1/threads/$script:threadId/posts" `
            -Body (@{ body = 'csrf' } | ConvertTo-Json) -ContentType 'application/json' `
            -Headers @{ Origin = 'https://evil.example' } -WebSession $admin -ErrorAction Stop | Out-Null
        $false
    }
    catch {
        $_.Exception.Response.StatusCode.value__ -eq 403
    }
}

Write-Host ""
if ($script:failures -gt 0) {
    Write-Host "$($script:failures) of $($script:checks) checks FAILED" -ForegroundColor Red
    exit 1
}
Write-Host "all $($script:checks) checks passed" -ForegroundColor Green
exit 0

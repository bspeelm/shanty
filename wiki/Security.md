# Security

shanty sits between a server you trust and a terminal you use, holding a
credential the whole time. The decisions below were written before the code, in
ADR-001, and each has a test.

## What a stolen `credentials.toml` yields

| what is in it | what a thief gets |
|---|---|
| an API key | one revocable string; you kill it server-side in a click |
| a token and salt | a credential that replays against this server and nowhere else |
| a `password_file` path | a path, and whatever your own tooling protects it with |
| a plaintext password | whatever else you reused that password for |

That ordering is why `shanty setup` asks for an API key first, writes a token
when you give it a password, and never writes the password itself.

The file is 0600 or shanty refuses to start.

## What another account on your machine can see

Not your credential.

Stream URLs carry the credential in a query parameter, and a URL on a command
line is readable by everyone through `ps`. shanty starts one mpv and hands it
every track over a unix socket instead, so the URL is in mpv's memory and
nowhere inspectable. The socket lives in a 0700 directory.

A test spawns a real child process and reads its `/proc/<pid>/cmdline` and
`environ` to prove it. It plants a credential in the parent's environment first,
to prove the search is looking somewhere.

## What a hostile or compromised server can do

It chooses every byte shanty parses. So:

- **JSON** is read through a capped reader and decoded by one function, which
  is fuzzed.
- **Titles and names** are stripped of anything a terminal would obey before
  they are drawn. A terminal is an interpreter, not a display: escape sequences
  move the cursor, clear the screen, and on many emulators ask a question whose
  answer is injected as if you had typed it. This is the cheapest attack a
  server has and it needs no bug to work.
- **Audio never enters shanty at all.** mpv fetches the stream itself.

Text is stripped where it is drawn, not where it is decoded, so the scrobble
shanty sends back describes the track the server actually has.

## What the network can do

Delay forever, or lie. Every HTTP client has a timeout, and response bodies are
read through a capped reader — a hostile server should cost a failed request,
not the machine's memory.

**TLS verification has no off switch.** Not a flag, not a config key, not an
environment variable. A self-signed certificate goes in your trust store, which
is the mechanism that already exists for it. A knob that weakens verification
is one paste away from living in every setup guide, and a knob that exists gets
turned.

## Where shanty connects

Your server. That is the whole list — no update checks, no telemetry, no
lyrics service, no scrobbling service. Exactly one package in the program
imports `net/http`, and a budget check asserts that.

Scrobbles go to your server's own endpoint. If you want them forwarded to
something else, that is a setting on the server you already administer, rather
than a second credential store on every machine you install this on.

## Releases

Every release carries `checksums.txt` and a Sigstore attestation built by the
workflow, with no private key anywhere. The installer checks the checksum
always and the attestation on `--verify`, and says which of the two it did.

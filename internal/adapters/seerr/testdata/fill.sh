#!/bin/sh
# Fills the scratch Seerr of compose.yaml (ADR 0008): its Jellyfin gets an
# owner and two invented people, Kim and Sam, and an empty library; Seerr
# signs in with Jellyfin's owner and imports the two. Kim asks for two
# public-domain films and Sam for a third and a show, all waiting for
# approval, but one the owner approves and one declines; Sam reports a
# problem with the approved film. It prints Seerr's API key for record.sh.
set -eu
j=http://localhost:8097
s=http://localhost:5055
client='MediaBrowser Client="fill", Device="fill", DeviceId="fill", Version="1"'
jf() { curl -sf -X POST -H "Content-Type: application/json" -H "Authorization: $client${token:+, Token=\"$token\"}" "$j$1" -d "$2"; }

# Jellyfin's first run, its owner and the two people, once both answer.
until curl -sf -o /dev/null "$j/Startup/User" && curl -sf -o /dev/null "$s/api/v1/status"; do sleep 2; done
jf /Startup/Configuration '{"UICulture":"en-GB","MetadataCountryCode":"GB","PreferredMetadataLanguage":"en"}' >/dev/null
curl -sf "$j/Startup/User" >/dev/null
jf /Startup/User '{"Name":"owner","Password":"owner-password"}' >/dev/null
jf /Startup/RemoteAccess '{"EnableRemoteAccess":true,"EnableAutomaticPortMapping":false}' >/dev/null
jf /Startup/Complete '{}' >/dev/null
token=$(jf /Users/AuthenticateByName '{"Username":"owner","Pw":"owner-password"}' | jq -r .AccessToken)
jf "/Library/VirtualFolders?name=Films&collectionType=movies&paths=%2Ftmp&refreshLibrary=false" '{"LibraryOptions":{}}' >/dev/null
for who in Kim Sam; do jf /Users/New "{\"Name\":\"$who\",\"Password\":\"$who-password\"}" >/dev/null; done

# Seerr's first run, signed in as Jellyfin's owner.
jar=$(mktemp)
as() { # cookie-jar method path [body]
  curl -sf -b "$1" -c "$1" -X "$2" -H "Content-Type: application/json" "$s/api/v1$3" ${4:+-d "$4"}
}
as "$jar" POST /auth/jellyfin '{"username":"owner","password":"owner-password","hostname":"jellyfin","port":8096,"useSsl":false,"urlBase":"","email":"owner@example.com","serverType":2}' >/dev/null
for id in $(as "$jar" POST /settings/jellyfin/library/sync '{}' | jq -r '.[].id'); do
  as "$jar" PUT "/settings/jellyfin/library/$id" '{"enabled":true}' >/dev/null
done
as "$jar" POST /settings/initialize '{}' >/dev/null
people=$(as "$jar" GET /settings/jellyfin/users | jq -c '[.[] | select(.username != "owner") | .id]')
# They may ask for things and report problems with them (32 and 4194304,
# as Seerr numbers its permissions), but nothing is approved by itself.
for id in $(as "$jar" POST /user/import-from-jellyfin "{\"jellyfinUserIds\":$people}" | jq '.[].id'); do
  as "$jar" POST "/user/$id/settings/permissions" '{"permissions":4194336}' >/dev/null
done

# Each one's requests, as themselves. A film by its TMDB id, as Seerr finds
# it there.
ask() { # person body
  pj=$(mktemp)
  as "$pj" POST /auth/jellyfin "{\"username\":\"$1\",\"password\":\"$1-password\"}" >/dev/null
  as "$pj" POST /request "$2" | jq .id
  rm "$pj"
}
ask Kim '{"mediaType":"movie","mediaId":653}' >/dev/null
ask Kim '{"mediaType":"movie","mediaId":19}' >/dev/null
approved=$(ask Sam '{"mediaType":"movie","mediaId":961}')
declined=$(ask Sam '{"mediaType":"movie","mediaId":10331}')
ask Sam '{"mediaType":"tv","mediaId":6357,"seasons":[1]}' >/dev/null
as "$jar" POST "/request/$approved/approve" '{}' >/dev/null
as "$jar" POST "/request/$declined/decline" '{}' >/dev/null

# Sam reports that the approved film's picture freezes.
media=$(as "$jar" GET "/request/$approved" | jq .media.id)
sp=$(mktemp)
as "$sp" POST /auth/jellyfin '{"username":"Sam","password":"Sam-password"}' >/dev/null
as "$sp" POST /issue "{\"issueType\":1,\"message\":\"The picture freezes ten minutes in.\",\"mediaId\":$media}" >/dev/null
rm "$sp"

as "$jar" GET /settings/main | jq -r .apiKey
rm "$jar"

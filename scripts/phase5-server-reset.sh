#!/bin/sh
set -eu

CONFIRMATION=--confirm-destroy-all-mindcreek-data
MODE=${1:---list}
PROJECTS="mykb-phase0 mindcreek-stage1 mindcreek-phase1 mindcreek-phase3 mindcreek-phase4 mindcreek-phase5 weknora"

usage() {
  cat >&2 <<EOF
usage:
  $0 --list
  $0 $CONFIRMATION

The confirmation mode permanently removes containers, attached anonymous
volumes, named volumes, and networks owned by known MyKB/MindCreek Compose
projects. Images, archives, repository files, and .local configuration remain.
EOF
  exit 2
}

command -v docker >/dev/null 2>&1 || { echo "docker is required" >&2; exit 2; }
docker info >/dev/null 2>&1 || { echo "the Docker daemon is unavailable" >&2; exit 2; }

list_resources() {
  for project in $PROJECTS; do
    containers=$(docker ps --all --quiet --filter "label=com.docker.compose.project=$project")
    volumes=$(docker volume ls --quiet --filter "label=com.docker.compose.project=$project")
    networks=$(docker network ls --quiet --filter "label=com.docker.compose.project=$project")
    if [ -n "$containers$volumes$networks" ]; then
      echo "Compose project: $project"
      docker ps --all --filter "label=com.docker.compose.project=$project" \
        --format '  container {{.Names}} ({{.Status}})'
      docker volume ls --filter "label=com.docker.compose.project=$project" \
        --format '  volume    {{.Name}}'
      docker network ls --filter "label=com.docker.compose.project=$project" \
        --format '  network   {{.Name}}'
    fi
  done
}

case "$MODE" in
  --list)
    list_resources
    exit 0
    ;;
  "$CONFIRMATION") ;;
  *) usage ;;
esac

echo "Destroying runtime data for known MyKB/MindCreek Compose projects"
list_resources

for project in $PROJECTS; do
  docker ps --all --quiet --filter "label=com.docker.compose.project=$project" | while IFS= read -r container; do
    [ -n "$container" ] && docker rm --force --volumes "$container"
  done
done

for project in $PROJECTS; do
  docker volume ls --quiet --filter "label=com.docker.compose.project=$project" | while IFS= read -r volume; do
    [ -n "$volume" ] && docker volume rm "$volume"
  done
done

for project in $PROJECTS; do
  docker network ls --quiet --filter "label=com.docker.compose.project=$project" | while IFS= read -r network; do
    [ -n "$network" ] && docker network rm "$network"
  done
done

remaining=0
for project in $PROJECTS; do
  if [ -n "$(docker ps --all --quiet --filter "label=com.docker.compose.project=$project")" ] || \
     [ -n "$(docker volume ls --quiet --filter "label=com.docker.compose.project=$project")" ] || \
     [ -n "$(docker network ls --quiet --filter "label=com.docker.compose.project=$project")" ]; then
    echo "reset incomplete for Compose project: $project" >&2
    remaining=1
  fi
done
[ "$remaining" -eq 0 ] || exit 1

anonymous=$(docker volume ls --quiet --filter dangling=true --filter label=com.docker.volume.anonymous)
if [ -n "$anonymous" ]; then
  echo "Warning: unrelated or untraceable dangling anonymous volumes were preserved:" >&2
  printf '%s\n' "$anonymous" >&2
fi

echo "Known MyKB/MindCreek server runtime data removed; images and configuration preserved"

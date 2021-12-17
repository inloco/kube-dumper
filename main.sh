#!/bin/bash
set -e

read -d '' JQ_SANITIZER << EOF || true
del(
    .metadata.annotations."autoscaling.alpha.kubernetes.io/conditions",
    .metadata.annotations."autoscaling.alpha.kubernetes.io/current-metrics",
    .metadata.annotations."control-plane.alpha.kubernetes.io/leader",
    .metadata.annotations."deployment.kubernetes.io/revision",
    .metadata.annotations."deployment.kubernetes.io/revision-history",
    .metadata.annotations."deployment.kubernetes.io/desired-replicas",
    .metadata.annotations."deployment.kubernetes.io/max-replicas",
    .metadata.annotations."kubectl.kubernetes.io/last-applied-configuration",
    .metadata.creationTimestamp,
    .metadata.deletionTimestamp,
    .metadata.finalizers,
    .metadata.generateName,
    .metadata.generation,
    .metadata.labels."pod-template-hash",
    .metadata.managedFields,
    .metadata.resourceVersion,
    .metadata.selfLink,
    .metadata.uid,
    .spec.nodeName,
    .spec.renewTime,
    .status
)
EOF

function sanitize {
    jq "$JQ_SANITIZER"
}

function json2yaml {
    yq -y .
}

function yaml2json {
    yq .
}

function owned {
    cat $* | yaml2json | jq -e .metadata.ownerReferences >/dev/null 2>&1
}

function differ {
    git add $* || true
    [ ! -z "$(git diff HEAD $*)" ]
}

function run_with_retry {
    parallel --retries 5 --delay 10 ::: "$*"
}

function log {
    echo "[$(date -u '+%F %T')] $*"
}

GLOBAL_RESOURCE_TYPES=$(kubectl api-resources --namespaced=false --output=name --verbs=create,get)
NAMESPACED_RESOURCE_TYPES=$(kubectl api-resources --namespaced=true --output=name --verbs=create,get)
NAMESPACES=$(kubectl get namespaces --output=name | cut -d / -f 2)

run_with_retry "git clone --depth 1 $CODECOMMIT_HTTPS ."
run_with_retry "git fetch --unshallow"
git config --local include.path ../.gitconfig

rm -fR *

mkdir -p _
cd _
    log "Handle global resource types"
    for RESOURCE_TYPE in $GLOBAL_RESOURCE_TYPES
    do
        log "Handle '${RESOURCE_TYPE}' global resource type"
        if [ "$RESOURCE_TYPE" == 'nodes' ]
        then
            log "'${RESOURCE_TYPE}' global resource type ignored"
            continue
        fi

        mkdir -p $RESOURCE_TYPE
        cd $RESOURCE_TYPE

        log "Request all resources of '${RESOURCE_TYPE}' global resource type"
        RESOURCES=$(kubectl get $RESOURCE_TYPE --output=json | jq -c '.items[]')
        echo -E "$RESOURCES" | while read -r RESOURCE
        do
            NAME=$(echo -E "$RESOURCE" | jq -r .metadata.name)
            log "Handle '${NAME}' resource of '${RESOURCE_TYPE}' global resource type"
            echo -E "$RESOURCE" | sanitize | json2yaml > $NAME.yaml
        done

        cd ..
    done
cd ..
log "Finished handle global resource types"

log "Handle namespaced resource types"
for NAMESPACE in $NAMESPACES
do
    log "Handle resources in namespace '${NAMESPACE}'"
    if [ "$NAMESPACE" == 'kube-node-lease' ]
    then
        log "'${NAMESPACE}' namespace ignored"
        continue
    fi

    mkdir -p $NAMESPACE
    cd $NAMESPACE

    for RESOURCE_TYPE in $NAMESPACED_RESOURCE_TYPES
    do
        log "Handle '${RESOURCE_TYPE}' resource type"
        if [ "$RESOURCE_TYPE" == 'events' ] || [ "$RESOURCE_TYPE" == 'events.events.k8s.io' ]
        then
            log "'${RESOURCE_TYPE}' resource type ignored"
            continue
        fi

        mkdir -p $RESOURCE_TYPE
        cd $RESOURCE_TYPE

        RESOURCES=$(kubectl get $RESOURCE_TYPE --namespace=$NAMESPACE --output=json | jq -c '.items[]')
        echo -E "$RESOURCES" | while read -r RESOURCE
        do
            NAME=$(echo -E "$RESOURCE" | jq -r .metadata.name)
            log "Handle '${NAME}' resource of '${RESOURCE_TYPE}' resource type"

            if [ "$NAMESPACE" == 'kube-system' ] && [ "$RESOURCE_TYPE" == 'configmaps' ] && [ "$NAME" == 'cluster-autoscaler-status' ]
            then
                log "'${NAME}' resource of '${RESOURCE_TYPE}' type from '${NAMESPACE}' namespace ignored"
                continue
            fi

            echo -E "$RESOURCE" | sanitize | json2yaml > $NAME.yaml

            if [ "$RESOURCE_TYPE" == 'secrets' ]
            then
                log "Encrypt '${NAME}' secret"
                sops -e -i $NAME.yaml

                if ! differ $NAME.yaml
                then
                    log "No difference found since previous version of '${NAME}' secret, discard encryption metadata change"
                    git checkout HEAD $NAME.yaml
                fi
            fi

            if owned $NAME.yaml
            then
                log "'${NAME}' resource ignored, is owned"
                rm $NAME.yaml
            fi
        done

        cd ..
    done

    cd ..
done
log "Finished handling namespaced resource types"

log "Commit changes"
git add -A
if git commit -m "$(date)"
then
    log "Push commit"
    run_with_retry "git push"
fi

log "Job completed"

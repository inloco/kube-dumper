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
    jq "${JQ_SANITIZER}"
}

function json2yaml {
    yq -y .
}

function owned {
    cat ${*} | yq -e .metadata.ownerReferences >/dev/null 2>&1
}

function differ {
    git add ${*} || true
    [ ! -z "$(git diff HEAD ${*})" ]
}

function retry {
    parallel --lb --delay 30 --retries 10 ::: "${*}"
}

function log {
    echo "[$(date -u +'%Y-%m-%dT%H:%M:%SZ')] ${*}"
}

log 'BEGIN - Kube Dumper'

log '+ KUBECTL - api-resources --namespaced=false'
UNNAMESPACED_TYPES=$(kubectl api-resources --namespaced=false --output=name --verbs=create,get)

log '+ KUBECTL - api-resources --namespaced=true'
NAMESPACED_TYPES=$(kubectl api-resources --namespaced=true --output=name --verbs=create,get)

log '+ KUBECTL - get namespaces'
NAMESPACES=$(kubectl get namespaces --output=name | cut -d / -f 2)

log '+ GIT - clone'
retry "git clone ${CODECOMMIT_HTTPS} ."

log '+ GIT - config'
git config --local include.path ../.gitconfig

log '+ RM - *'
rm -fR *

log '+ BEGIN - Unnamespaced Resources'
    log '++ MKDIR - _'
    mkdir -p _

    log '++ CD - _'
    cd _
        for TYPE in ${UNNAMESPACED_TYPES}
        do
            if [ -z "${TYPE}" ]
            then
                continue
            fi

            if [ "${TYPE}" == 'nodes' ]
            then
                log "+++ IGNORE - TYPE ${TYPE}"
                continue
            fi

            log "+++ BEGIN - TYPE ${TYPE}"
                log "++++ KUBECTL - get ${TYPE}"
                RESOURCES=$(kubectl get "${TYPE}" --output=json | jq -c '.items[]')

                log "++++ MKDIR - ${TYPE}"
                mkdir -p "${TYPE}"

                log "++++ CD - ${TYPE}"
                cd "${TYPE}"
                    echo -E "${RESOURCES}" | while read -r RESOURCE
                    do
                        if [ -z "${RESOURCE}" ]
                        then
                            continue
                        fi

                        NAME=$(echo -En "${RESOURCE}" | jq -r .metadata.name)

                        log "+++++ BACKUP - RESOURCE ${TYPE}/${NAME}"
                        echo -En "${RESOURCE}" | sanitize | json2yaml > "${NAME}.yaml"
                    done
                cd ..
                log '++++ CD - ..'
            log "+++ END - TYPE ${TYPE}"
        done
    cd ..
    log '++ CD - ..'
log '+ END - Unnamespaced Resources'

log '+ BEGIN - Namespaced Resources'
for NAMESPACE in ${NAMESPACES}
do
    if [ -z "${NAMESPACE}" ]
    then
        continue
    fi

    if [ "${NAMESPACE}" == 'kube-node-lease' ]
    then
        log "++ IGNORE - NAMESPACE ${NAMESPACE}"
        continue
    fi

    log "++ BEGIN - NAMESPACE ${NAMESPACE}"
        log "+++ MKDIR - ${NAMESPACE}"
        mkdir -p "${NAMESPACE}"

        log "+++ CD - ${NAMESPACE}"
        cd "${NAMESPACE}"
            for TYPE in ${NAMESPACED_TYPES}
            do
                if [ -z "${TYPE}" ]
                then
                    continue
                fi

                if [ "${TYPE}" == 'events' ] || [ "${TYPE}" == 'events.events.k8s.io' ]
                then
                    log "++++ IGNORE - TYPE ${TYPE}"
                    continue
                fi

                log "++++ BEGIN - TYPE ${TYPE}"
                    log "+++++ KUBECTL - get ${TYPE} --namespace=${NAMESPACE}"
                    RESOURCES=$(kubectl get "${TYPE}" --namespace="${NAMESPACE}" --output=json | jq -c '.items[]')

                    log "+++++ MKDIR - ${TYPE}"
                    mkdir -p "${TYPE}"

                    log "+++++ CD - ${TYPE}"
                    cd "${TYPE}"
                        echo -E "${RESOURCES}" | while read -r RESOURCE
                        do
                            if [ -z "${RESOURCE}" ]
                            then
                                continue
                            fi

                            NAME=$(echo -En "${RESOURCE}" | jq -r .metadata.name)

                            if [ "${NAME}" == 'cluster-autoscaler-status' ] && [ "${TYPE}" == 'configmaps' ] && [ "${NAMESPACE}" == 'kube-system' ]
                            then
                                log "++++++ IGNORE - RESOURCE --namespace=${NAMESPACE} ${TYPE}/${NAME}"
                                continue
                            fi

                            log "++++++ BACKUP - RESOURCE --namespace=${NAMESPACE} ${TYPE}/${NAME}"
                            echo -En "${RESOURCE}" | sanitize | json2yaml > "${NAME}.yaml"

                            if [ "${TYPE}" == 'secrets' ]
                            then
                                log "++++++ ENCRYPT - RESOURCE --namespace=${NAMESPACE} ${TYPE}/${NAME}"
                                sops -e -i "${NAME}.yaml"

                                if ! differ "${NAME}.yaml"
                                then
                                    log "++++++ SKIP EQUALS - RESOURCE --namespace=${NAMESPACE} ${TYPE}/${NAME}"
                                    git checkout HEAD "${NAME}.yaml"
                                fi
                            fi

                            if owned "${NAME}.yaml"
                            then
                                log "++++++ SKIP OWNED - RESOURCE --namespace=${NAMESPACE} ${TYPE}/${NAME}"
                                rm -f "${NAME}.yaml"
                            fi
                        done
                    cd ..
                    log '+++++ CD - ..'
                log "++++ END - TYPE ${TYPE}"
            done
        cd ..
        log '+++ CD - ..'
    log "++ END - NAMESPACE ${NAMESPACE}"
done
log '+ END - Namespaced Resources'

log '+ GIT - add'
git add -A

log '+ GIT - commit'
if git commit -m "$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
then
    log '+ GIT - push'
    retry 'git push'
fi

log 'END - Kube Dumper'

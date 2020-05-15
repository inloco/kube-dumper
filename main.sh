#!/bin/sh
set -ex

read -d '' JQ_SANITIZER << EOF || true
del(
    .metadata.annotations."control-plane.alpha.kubernetes.io/leader",
    .metadata.annotations."deployment.kubernetes.io/revision",
    .metadata.annotations."deployment.kubernetes.io/revision-history",
    .metadata.annotations."deployment.kubernetes.io/desired-replicas",
    .metadata.annotations."deployment.kubernetes.io/max-replicas",
    .metadata.annotations."kubectl.kubernetes.io/last-applied-configuration:",
    .metadata.creationTimestamp,
    .metadata.deletionTimestamp,
    .metadata.finalizers,
    .metadata.generateName,
    .metadata.generation,
    .metadata.labels."pod-template-hash",
    .metadata.resourceVersion,
    .metadata.selfLink,
    .metadata.uid,
    .status,
    .spec.nodeName
)
EOF

function sanitize {
    jq "$JQ_SANITIZER"
}

function json2yaml {
    ruby -ryaml -rjson -e "puts YAML.dump(JSON.parse(STDIN.read))" | tail -n +2
}

function yaml2json {
    ruby -ryaml -rjson -e "puts JSON.generate(YAML.load(STDIN.read))"
}

function owned {
    cat $* | yaml2json | jq -e .metadata.ownerReferences >/dev/null 2>&1
}

function differ {
    git add $* || true
    [ ! -z "$(git diff HEAD $*)" ]
}

GLOBAL_RESOURCE_TYPES=$(kubectl api-resources --namespaced=false --output=name --verbs=create,get)
NAMESPACED_RESOURCE_TYPES=$(kubectl api-resources --namespaced=true --output=name --verbs=create,get)
NAMESPACES=$(kubectl get namespaces -o name | cut -d / -f 2)

git clone --depth 1 $CODECOMMIT_HTTPS .
git config --local include.path ../.gitconfig

rm -fR *

mkdir -p _
cd _
    for RESOURCE_TYPE in $GLOBAL_RESOURCE_TYPES
    do
        if [ "$RESOURCE_TYPE" == 'nodes' ]
        then
            continue
        fi

        mkdir -p $RESOURCE_TYPE
        cd $RESOURCE_TYPE

        RESOURCES=$(kubectl get $RESOURCE_TYPE --output=name | cut -d / -f 2)
        for RESOURCE in $RESOURCES
        do
            kubectl get $RESOURCE_TYPE/$RESOURCE --output=json | sanitize | json2yaml > $RESOURCE.yaml
        done

        cd ..
    done
cd ..

for NAMESPACE in $NAMESPACES
do
    if [ "$NAMESPACE" == 'kube-node-lease' ]
    then
        continue
    fi

    mkdir -p $NAMESPACE
    cd $NAMESPACE

    for RESOURCE_TYPE in $NAMESPACED_RESOURCE_TYPES
    do
        if [ "$RESOURCE_TYPE" == 'events' ] || [ "$RESOURCE_TYPE" == 'events.events.k8s.io' ]
        then
            continue
        fi

        mkdir -p $RESOURCE_TYPE
        cd $RESOURCE_TYPE

        RESOURCES=$(kubectl get $RESOURCE_TYPE --namespace=$NAMESPACE --output=name | cut -d / -f 2)
        for RESOURCE in $RESOURCES
        do
            kubectl get $RESOURCE_TYPE/$RESOURCE --namespace=$NAMESPACE --output=json | sanitize | json2yaml > $RESOURCE.yaml

            if [ "$RESOURCE_TYPE" == 'secrets' ]
            then
                sops -e -i $RESOURCE.yaml

                if ! differ $RESOURCE.yaml
                then
                    git checkout HEAD $RESOURCE.yaml
                fi
            fi

            if owned $RESOURCE.yaml
            then
                rm $RESOURCE.yaml
            fi
        done

        cd ..
    done

    cd ..
done

git add -A
if git commit -m "$(date)"
then
    git push
fi

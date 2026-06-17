#!/bin/bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FABRIC_DIR="$(cat /etc/fabric_dir.conf)"
K3S_SERVICE_FILE="/etc/systemd/system/k3s.service"
FLANNEL_IFACE_LINE="--flannel-iface tap0 \\"

# Função para reinstalar k3s (para resolver bugs de instalação de chaincode)
function k3s() {
  echo "[ALERT] Caso tenha usado esse comando com a rede de pé, reinicie a rede antes de prosseguir"
  sleep 2
  echo "[INFO] Iniciando desinstalação do K3S"
  /usr/local/bin/k3s-uninstall.sh
  /usr/local/bin/k3s-agent-uninstall.sh

  echo "[INFO] Instalando K3S novamente"
  sudo curl -k -sfL https://get.k3s.io | sh -s - --disable traefik --write-kubeconfig-mode 644 --node-name k3s-master-01
  sudo systemctl daemon-reload
  echo "[INFO] K3S desinstalado com sucesso"
}

# Funções de configuração de interface para o k3s
tap0Exists() { # verifica se a interface dummy existe e cria caso não exista
  if ! ip link show tap0 &> /dev/null; then
    echo "[INFO] Criando interface dummy tap0..."
    sudo ip tuntap add mode tap tap0
    sudo ip addr add 10.243.255.254/24 dev tap0
  fi
}

tap0Up() { # levanta a interface dummy
  sudo ip link set tap0 up
}

tap0Down() { # derruba a interface dummy
  if ip link show tap0 &> /dev/null; then
    echo "[INFO] Desativando interface dummy..."
    sudo ip link set tap0 down
  fi
}

addFlannel() {
  local iface="$1"
  local temp_file=$(mktemp)

  # Verifica se já existe
  if grep -q "'--flannel-iface'" "$K3S_SERVICE_FILE"; then
    echo "[INFO] Interface já configurada no serviço k3s."
    return
  fi

  echo "[INFO] Adicionando interface $iface ao serviço do k3s..."

  # Processa o arquivo com awk para garantir formatação perfeita
  awk -v iface="$iface" '
    /'\''--node-name'\''/ {
      print $0
      getline
      print $0
      print "\t'\''--flannel-iface'\'' \\"
      print "\t'\''" iface "'\'' \\"
      next
    }
    { print }
  ' "$K3S_SERVICE_FILE" > "$temp_file"

  # Substitui o arquivo original
  sudo mv "$temp_file" "$K3S_SERVICE_FILE"
  sudo chmod 644 "$K3S_SERVICE_FILE"
  sudo systemctl daemon-reload

  echo "[SUCCESS] Configuração adicionada com formatação correta!"
}

removeFlannel() { # remove a configuração da interface do serviço do k3s e reinicia o serviço
  if grep -q "'--flannel-iface'" "$K3S_SERVICE_FILE"; then
    echo "[INFO] Removendo interface dummy do serviço do k3s..."
    sudo sed -i "/'--flannel-iface'/,+1 d" "$K3S_SERVICE_FILE"
    sudo systemctl daemon-reload
  fi
}

################################ Funções de gerenciamento da rede ################################

: '
O script "up" faz o levantamento completo da rede, criando o cluster k3s e instalando várias dependencias recicláveis, como o 
istioctl, hlf-operator, registry docker, etc. Esse comando precisa ser usado no primeiro levantamento ou após usar o 
comando shutdown, pois essas dependências recicláveis não estarão disponíveis.
'

function up() { 
  echo "Esse script faz o levantamento offline. Todas as dependências devem ser instaladas."
  echo "Caso não tenha feito ainda, execute o install.sh e execute novamente esse script."
  echo "Se precisar de imagens atualizadas, execute o old_loadbalancer.sh."
  echo ""
  
  echo "[INFO] Verificando interface ativa..."
  DEFAULT_IFACE=$(ip route | grep '^default' | awk '{print $5}')
  IFACE_STATUS=""

  USE_CUSTOM=false
  CUSTOM_IFACE="tailscale0"

  if [[ "$USE_CUSTOM" == "true" ]]; then
    # Se estiver usando interface customizada
    echo "[INFO] Usando interface customizada $CUSTOM_IFACE"
    addFlannel "$CUSTOM_IFACE"
  else
    # Remove qualquer configuração anterior de flannel
    removeFlannel
  
    # Verifica a interface padrão
    if [[ -n "$DEFAULT_IFACE" ]]; then
      IFACE_STATUS=$(cat /sys/class/net/$DEFAULT_IFACE/operstate 2>/dev/null)
    fi

    if [[ -z "$DEFAULT_IFACE" || "$IFACE_STATUS" != "up" ]]; then
      echo "[INFO] Nenhuma interface default operante. Usando interface Dummy"
      tap0Exists
      tap0Up
      addFlannel "tap0"
    else
      echo "[INFO] Interface $DEFAULT_IFACE está operante. Ignorando interface Dummy"
      tap0Down
      removeFlannel
    fi
  fi

  echo "reiniciando k3s"
  sudo systemctl restart k3s --now
  sudo systemctl daemon-reload

  IMG_DIR=$FABRIC_DIR/fabric-offline/images

  echo "iniciando processo de levantamento da rede"
  if [ ! -d "$HOME/.kube" ]; then
  mkdir -p "$HOME/.kube"
  fi

  sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config

  echo "Carregando imagens istio"
  docker load -i $IMG_DIR/istio-pilot-1.26.0.tar
  docker load -i $IMG_DIR/istio-proxyv2-1.26.0.tar

  istioctl install --set profile=default -y \
    --set hub=docker.io/istio \
    --set tag=1.26.0

  kubectl apply -f - <<EOF
apiVersion: install.istio.io/v1alpha1
kind: IstioOperator
metadata:
  name: istio-gateway
  namespace: istio-system
spec:
  addonComponents:
    grafana:
      enabled: false
    kiali:
      enabled: false
    prometheus:
      enabled: false
    tracing:
      enabled: false
  components:
    ingressGateways:
      - enabled: true
        k8s:
          hpaSpec:
            minReplicas: 2
          resources:
            limits:
              cpu: 500m
              memory: 512Mi
            requests:
              cpu: 100m
              memory: 128Mi
          service:
            ports:
              - name: http
                port: 80
                targetPort: 8080
                nodePort: 30949
              - name: https
                port: 443
                targetPort: 8443
                nodePort: 30950
            type: LoadBalancer
        name: istio-ingressgateway
    pilot:
      enabled: true
      k8s:
        hpaSpec:
          minReplicas: 1
        resources:
          limits:
            cpu: 300m
            memory: 512Mi
          requests:
            cpu: 100m
            memory: 128Mi
  meshConfig:
    accessLogFile: /dev/stdout
    enableTracing: false
    outboundTrafficPolicy:
      mode: ALLOW_ANY
  profile: default

EOF

  sleep 2

  HLF_OP_DIR="$FABRIC_DIR/fabric-offline/hlf-operator"
  
  helm upgrade --install hlf-operator --version=1.13.0 -- "${HLF_OP_DIR}/hlf-operator-1.13.0.tgz"
    #  --namespace hlf-operator --create-namespace --set installCRDs=true \
    #  --set kube-rbac-proxy.enabled=true --set kube-rbac-proxy.repository="quay.io/brancz/kube-rbac-proxy" \
    #  --set kube-rbac-proxy.tag="v0.14.1" --wait --timeout=120s

  # helm repo add kfs https://kfsoftware.github.io/hlf-helm-charts --force-update
  # helm upgrade --install hlf-operator --version=1.11.1 -- kfs/hlf-operator
  # # instala o plugin hlf ao kubectl
  # kubectl krew install hlf

  CLUSTER_IP=$(kubectl get svc istio-ingressgateway -n istio-system -o json | jq -r '.status.loadBalancer.ingress[0].ip')
  HOSTNAMES=(
  "inmetro-ca.inmetro.br"
  "peer0.inmetro.br"
  "ord-ca.inmetro.br"
  "orderer0-ord.inmetro.br"
  "orderer1-ord.inmetro.br"
  "orderer2-ord.inmetro.br"
  "admin-orderer0-ord.inmetro.br"
  "admin-orderer1-ord.inmetro.br"
  "admin-orderer2-ord.inmetro.br"
  )

  for HOST in "${HOSTNAMES[@]}"; do
    sudo sed -i "/$HOST/d" /etc/hosts
    echo "$CLUSTER_IP $HOST" | sudo tee -a /etc/hosts > /dev/null
  done

    kubectl apply -f - <<EOF
  kind: ConfigMap
  apiVersion: v1
  metadata:
    name: coredns
    namespace: kube-system
  data:
    Corefile: |
      .:53 {
          errors
          health {
            lameduck 5s
          } 
        rewrite name regex (.*)\.localho\.st istio-ingressgateway.istio-system.svc.cluster.local
        rewrite name regex (.*)\.\${CLUSTER_IP//./\\.} istio-ingressgateway.istio-system.svc.cluster.local
        rewrite name regex (.*)\.inmetro\.br istio-ingressgateway.istio-system.svc.cluster.local
        rewrite name regex (.*)\.puc\.br istio-ingressgateway.istio-system.svc.cluster.local          
        rewrite name regex (.*)\.ord\.br istio-ingressgateway.istio-system.svc.cluster.local                  
          hosts {
            ${CLUSTER_IP} istio-ingressgateway.istio-system.svc.cluster.local
            fallthrough
          }
          ready
          log . {
            class error
          }
          kubernetes cluster.local in-addr.arpa ip6.arpa {
            pods insecure
            fallthrough in-addr.arpa ip6.arpa
            ttl 30
          }
          prometheus :9153
          forward . /etc/resolv.conf {
            max_concurrent 1000
          }
          cache 30
          loop
          reload
          loadbalance
      }
EOF

  sleep 5

  docker run -d -p 5000:5000 --restart=always --name registry registry:2
  REGISTRY="localhost:5000"

  docker load -i $IMG_DIR/fabric-peer-3.1.0.tar
  docker load -i $IMG_DIR/fabric-orderer-3.1.0.tar
  docker load -i $IMG_DIR/fabric-ca-1.5.15.tar
  #docker load -i $IMG_DIR/kube-rbac-proxy-v0.14.1.tar
  
  docker tag hyperledger/fabric-peer:3.1.0 $REGISTRY/fabric-peer:3.1.0
  docker tag hyperledger/fabric-orderer:3.1.0 $REGISTRY/fabric-orderer:3.1.0
  docker tag hyperledger/fabric-ca:1.5.15 $REGISTRY/fabric-ca:1.5.15

  docker push $REGISTRY/fabric-peer:3.1.0
  docker push $REGISTRY/fabric-orderer:3.1.0
  docker push $REGISTRY/fabric-ca:1.5.15

  sleep 10

  # variaveis de ambiente
  export PEER_IMAGE="$REGISTRY/fabric-peer"
  export PEER_VERSION=3.1.0

  export ORDERER_IMAGE="$REGISTRY/fabric-orderer"
  export ORDERER_VERSION=3.1.0

  export CA_IMAGE="$REGISTRY/fabric-ca"
  export CA_VERSION=1.5.15

  export MSP_ORG=INMETROMSP
  export PEER_SECRET=peerpw
  export STORAGE_CLASS=local-path
  export DATABASE=leveldb 

  export K8S_BUILDER=false

  # Organização INMETRO

  kubectl hlf ca create  --image=$CA_IMAGE --version=$CA_VERSION --storage-class=$STORAGE_CLASS --capacity=1Gi --name=inmetro-ca \
      --enroll-id=enroll --enroll-pw=enrollpw --istio-port=443 --istio-ingressgateway=ingressgateway --hosts="inmetro-ca.inmetro.br" #--output=true > resources/inmetroca.yaml

  sleep 3

  # kubectl apply -f resources/inmetroca.yaml

  kubectl wait --timeout=180s --for=condition=Running fabriccas.hlf.kungfusoftware.es --all
  
  sleep 3

  kubectl hlf ca register --name=inmetro-ca --user=peer --secret=peerpw \
      --type=peer --enroll-id enroll --enroll-secret=enrollpw --mspid=INMETROMSP --ca-url="https://inmetro-ca.inmetro.br:443"

  sleep 5

  kubectl hlf peer create --statedb=$DATABASE --image=$PEER_IMAGE --version=$PEER_VERSION --storage-class=$STORAGE_CLASS --enroll-id=peer --mspid=$MSP_ORG \
  --enroll-pw=$PEER_SECRET --capacity=5Gi --name=inmetro-peer0 --ca-name=inmetro-ca.default --hosts=peer0.inmetro.br --istio-port=443 --istio-ingressgateway=ingressgateway \
  --k8s-builder=true
  
  sleep 5

  kubectl wait --timeout=180s --for=condition=Running fabricpeers.hlf.kungfusoftware.es --all

  echo "Criando 3 orderers"

  # register orderer user
  kubectl hlf ca create  --image=$CA_IMAGE --version=$CA_VERSION --storage-class=$STORAGE_CLASS --capacity=1Gi --name=ord-ca \
      --enroll-id=enroll --enroll-pw=enrollpw --hosts=ord-ca.inmetro.br --istio-ingressgateway=ingressgateway --istio-port=443

  sleep 4

  kubectl wait --timeout=180s --for=condition=Running fabriccas.hlf.kungfusoftware.es --all

  kubectl hlf ca register --name=ord-ca --user=orderer --secret=ordererpw \
      --type=orderer --enroll-id enroll --enroll-secret=enrollpw --mspid=OrdererMSP --ca-url="https://ord-ca.inmetro.br:443"
  sleep 2
  kubectl hlf ordnode create --image=$ORDERER_IMAGE --version=$ORDERER_VERSION \
      --storage-class=$STORAGE_CLASS --enroll-id=orderer --mspid=OrdererMSP \
      --enroll-pw=ordererpw --capacity=2Gi --name=ord-node0 --ca-name=ord-ca.default \
      --hosts=orderer0-ord.inmetro.br --istio-ingressgateway=ingressgateway --istio-port=443 --admin-hosts=admin-orderer0-ord.inmetro.br 
  kubectl hlf ordnode create --image=$ORDERER_IMAGE --version=$ORDERER_VERSION \
      --storage-class=$STORAG E_CLASS --enroll-id=orderer --mspid=OrdererMSP \
      --enroll-pw=ordererpw --capacity=2Gi --name=ord-node1 --ca-name=ord-ca.default \
      --hosts=orderer1-ord.inmetro.br --istio-ingressgateway=ingressgateway --istio-port=443 --admin-hosts=admin-orderer1-ord.inmetro.br
  kubectl hlf ordnode create --image=$ORDERER_IMAGE --version=$ORDERER_VERSION \
      --storage-class=$STORAGE_CLASS --enroll-id=orderer --mspid=OrdererMSP \
      --enroll-pw=ordererpw --capacity=2Gi --name=ord-node2 --ca-name=ord-ca.default \
      --hosts=orderer2-ord.inmetro.br --istio-ingressgateway=ingressgateway --istio-port=443 --admin-hosts=admin-orderer2-ord.inmetro.br

  kubectl wait --timeout=180s --for=condition=Running fabricorderernodes.hlf.kungfusoftware.es --all

  sleep 4
  ## register OrdererMSP Identity
  kubectl hlf ca register --name=ord-ca --user=admin --secret=adminpw \
      --type=admin --enroll-id enroll --enroll-secret=enrollpw --mspid=OrdererMSP #--ca-url="https://ord-ca.inmetro.br:443"

  # kubectl hlf ca enroll --name=ord-ca --namespace=default \
  #     --user=admin --secret=adminpw --mspid=OrdererMSP \
  #     --ca-name tlsca --output resources/orderermsp.yaml

  kubectl hlf identity create --name orderer-admin-sign --namespace default \
      --ca-name ord-ca --ca-namespace default \
      --ca ca --mspid OrdererMSP --enroll-id admin --enroll-secret adminpw # sign identity

  sleep 3

  kubectl hlf identity create --name orderer-admin-tls --namespace default \
      --ca-name ord-ca --ca-namespace default \
      --ca tlsca --mspid OrdererMSP --enroll-id admin --enroll-secret adminpw # tls identity

  sleep 3
  ## register INMETROMSP Identity
  # register
  kubectl hlf ca register --name=inmetro-ca --namespace=default --user=admin --secret=adminpw \
      --type=admin --enroll-id enroll --enroll-secret=enrollpw --mspid=INMETROMSP

  # enroll
  kubectl hlf identity create --name inmetro-admin --namespace default \
      --ca-name inmetro-ca --ca-namespace default \
      --ca ca --mspid INMETROMSP --enroll-id admin --enroll-secret adminpw

  sleep 2

  # criação de canal
  export PEER_ORG_SIGN_CERT=$(kubectl get fabriccas inmetro-ca -o=jsonpath='{.status.ca_cert}')
  export PEER_ORG_TLS_CERT=$(kubectl get fabriccas inmetro-ca -o=jsonpath='{.status.tlsca_cert}')
  export IDENT_8=$(printf "%8s" "")
  export ORDERER_TLS_CERT=$(kubectl get fabriccas ord-ca -o=jsonpath='{.status.tlsca_cert}' | sed -e "s/^/${IDENT_8}/" )
  export ORDERER0_TLS_CERT=$(kubectl get fabricorderernodes ord-node0 -o=jsonpath='{.status.tlsCert}' | sed -e "s/^/${IDENT_8}/" )
  export ORDERER1_TLS_CERT=$(kubectl get fabricorderernodes ord-node1 -o=jsonpath='{.status.tlsCert}' | sed -e "s/^/${IDENT_8}/" )
  export ORDERER2_TLS_CERT=$(kubectl get fabricorderernodes ord-node2 -o=jsonpath='{.status.tlsCert}' | sed -e "s/^/${IDENT_8}/" )

  kubectl apply -f - <<EOF
apiVersion: hlf.kungfusoftware.es/v1alpha1
kind: FabricMainChannel
metadata:
  name: demo
spec:
  name: demo
  adminOrdererOrganizations:
    - mspID: OrdererMSP
  adminPeerOrganizations:
    - mspID: INMETROMSP
  channelConfig:
    application:
      acls: null
      capabilities:
        - V2_0
      policies: null
    capabilities:
      - V2_0
    orderer:
      batchSize:
        absoluteMaxBytes: 1048576
        maxMessageCount: 10
        preferredMaxBytes: 524288
      batchTimeout: 2s
      capabilities:
        - V2_0
      etcdRaft:
        options:
          electionTick: 10
          heartbeatTick: 1
          maxInflightBlocks: 5
          snapshotIntervalSize: 16777216
          tickInterval: 500ms
      ordererType: etcdraft
      policies: null
      state: STATE_NORMAL
    policies: null
  externalOrdererOrganizations: []
  peerOrganizations:
    - mspID: INMETROMSP
      caName: "inmetro-ca"
      caNamespace: "default"
  identities:
    OrdererMSP:
      secretKey: user.yaml
      secretName: orderer-admin-tls
      secretNamespace: default
    OrdererMSP-sign:
      secretKey: user.yaml
      secretName: orderer-admin-sign
      secretNamespace: default
    INMETROMSP:
      secretKey: user.yaml
      secretName: inmetro-admin
      secretNamespace: default
  externalPeerOrganizations: []
  ordererOrganizations:
    - caName: "ord-ca"
      caNamespace: "default"
      externalOrderersToJoin:
        - host: ord-node0
          port: 7053
        - host: ord-node1
          port: 7053
        - host: ord-node2
          port: 7053
      mspID: OrdererMSP
      ordererEndpoints:
        - orderer0-ord.inmetro.br:443
        - orderer1-ord.inmetro.br:443
        - orderer2-ord.inmetro.br:443
      orderersToJoin: []
  orderers:
    - host: orderer0-ord.inmetro.br
      port: 443
      tlsCert: |-
${ORDERER0_TLS_CERT}
    - host: orderer1-ord.inmetro.br
      port: 443
      tlsCert: |-
${ORDERER1_TLS_CERT}
    - host: orderer2-ord.inmetro.br
      port: 443
      tlsCert: |-
${ORDERER2_TLS_CERT}
EOF

  echo "Aguardando canal ser carregado"
  sleep 15
  echo "ingressando peers da organização inmetro no canal"

  export IDENT_8=$(printf "%8s" "")
  export ORDERER0_TLS_CERT=$(kubectl get fabricorderernodes ord-node0 -o=jsonpath='{.status.tlsCert}' | sed -e "s/^/${IDENT_8}/" )

  kubectl apply -f - <<EOF
apiVersion: hlf.kungfusoftware.es/v1alpha1
kind: FabricFollowerChannel
metadata:
  name: demo-inmetromsp
spec:
  anchorPeers:
    - host: peer0.inmetro.br
      port: 443 
  hlfIdentity:
    secretKey: user.yaml
    secretName: inmetro-admin
    secretNamespace: default
  mspId: INMETROMSP
  name: demo
  externalPeersToJoin: []
  orderers:
    - certificate: |
${ORDERER0_TLS_CERT}
      url: grpcs://orderer0-ord.inmetro.br:443
  peersToJoin:
    - name: inmetro-peer0
      namespace: default
EOF

  sleep 5
  echo "Criando NetworkConfig e InmetroCP"

  kubectl hlf identity create --name inmetro-admin --namespace default \
      --ca-name inmetro-ca --ca-namespace default \
      --ca ca --mspid INMETROMSP --enroll-id explorer-admin --enroll-secret explorer-adminpw \
      --ca-enroll-id=enroll --ca-enroll-secret=enrollpw --ca-type=admin

  kubectl hlf networkconfig create --name=inmetro-cp \
      -o INMETROMSP -o OrdererMSP -c demo \
      --identities=inmetro-admin.default --secret=inmetro-cp
  echo ""
  sleep 2

  kubectl get pods
  echo "Rede levantada com sucesso!"

}

: '
Essa função faz o levantamento leve da rede, reciclando as dependências que o comando up instala. Deve ser usado em conjunto do comando down,
que faz o derrubamento da rede mantendo alguns itens operantes, agilizando o processo de levantamento. Caso algum erro seja encontrado, considere
fazer o processo de levantamento limpo, usando o shutdown e up, ou o comando restart, que reinicia a rede do zero.
!!! NÃO DEVE SER USADO JUNTO DO COMANDO SHUTDOWN, POIS RESULTARÁ EM ERRO" !!!
!!! NÃO DEVE SER USADO APÓS CRIAR O CLUSTER COM/SEM REDE E TER TROCADO A CONEXÃO (COM REDE>SEM REDE / SEM REDE>COM REDE) !!!
'

function lup() {
  echo "Esse script faz o levantamento "light" da rede, pulando algumas instalações. Caso"
  echo "seja a primeira vez que está levantando a rede, use o comando "up", que configura as"
  echo "dependências necessárias para o levantamento completo."

  kubectl apply -f - <<EOF
apiVersion: install.istio.io/v1alpha1
kind: IstioOperator
metadata:
  name: istio-gateway
  namespace: istio-system
spec:
  addonComponents:
    grafana:
      enabled: false
    kiali:
      enabled: false
    prometheus:
      enabled: false
    tracing:
      enabled: false
  components:
    ingressGateways:
      - enabled: true
        k8s:
          hpaSpec:
            minReplicas: 2
          resources:
            limits:
              cpu: 500m
              memory: 512Mi
            requests:
              cpu: 100m
              memory: 128Mi
          service:
            ports:
              - name: http
                port: 80
                targetPort: 8080
                nodePort: 30949
              - name: https
                port: 443
                targetPort: 8443
                nodePort: 30950
            type: LoadBalancer
        name: istio-ingressgateway
    pilot:
      enabled: true
      k8s:
        hpaSpec:
          minReplicas: 1
        resources:
          limits:
            cpu: 300m
            memory: 512Mi
          requests:
            cpu: 100m
            memory: 128Mi
  meshConfig:
    accessLogFile: /dev/stdout
    enableTracing: false
    outboundTrafficPolicy:
      mode: ALLOW_ANY
  profile: default

EOF

  sleep 2

  helm upgrade --install hlf-operator --version=1.11.1 "${HLF_OP_DIR}"

  CLUSTER_IP=$(kubectl get svc istio-ingressgateway -n istio-system -o json | jq -r '.status.loadBalancer.ingress[0].ip')
  HOSTNAMES=(
  "inmetro-ca.inmetro.br"
  "peer0.inmetro.br"
  "ord-ca.inmetro.br"
  "orderer0-ord.inmetro.br"
  "orderer1-ord.inmetro.br"
  "orderer2-ord.inmetro.br"
  "admin-orderer0-ord.inmetro.br"
  "admin-orderer1-ord.inmetro.br"
  "admin-orderer2-ord.inmetro.br"
  )

  for HOST in "${HOSTNAMES[@]}"; do
    sudo sed -i "/$HOST/d" /etc/hosts
    echo "$CLUSTER_IP $HOST" | sudo tee -a /etc/hosts > /dev/null
  done

  kubectl apply -f - <<EOF
kind: ConfigMap
apiVersion: v1
metadata:
  name: coredns
  namespace: kube-system
data:
  Corefile: |
    .:53 {
        errors
        health {
          lameduck 5s
        } 
        rewrite name regex (.*)\.localho\.st istio-ingressgateway.istio-system.svc.cluster.local
        rewrite name regex (.*)\.\${CLUSTER_IP//./\\.} istio-ingressgateway.istio-system.svc.cluster.local
        rewrite name regex (.*)\.inmetro\.br istio-ingressgateway.istio-system.svc.cluster.local
        rewrite name regex (.*)\.puc\.br istio-ingressgateway.istio-system.svc.cluster.local          
        rewrite name regex (.*)\.ord\.br istio-ingressgateway.istio-system.svc.cluster.local                 
        hosts {
          ${CLUSTER_IP} istio-ingressgateway.istio-system.svc.cluster.local
          fallthrough
        }
        ready
        log . {
          class error
        }
        kubernetes cluster.local in-addr.arpa ip6.arpa {
          pods insecure
          fallthrough in-addr.arpa ip6.arpa
          ttl 30
        }
        prometheus :9153
        forward . /etc/resolv.conf {
          max_concurrent 1000
        }
        cache 30
        loop
        reload
        loadbalance
    }
EOF

  sleep 5

  docker run -d -p 5000:5000 --restart=always --name registry registry:2
  REGISTRY="localhost:5000"

  sleep 3
  
  # variaveis de ambiente
  export PEER_IMAGE="$REGISTRY/fabric-peer"
  export PEER_VERSION=3.1.0

  export ORDERER_IMAGE="$REGISTRY/fabric-orderer"
  export ORDERER_VERSION=3.1.0

  export CA_IMAGE="$REGISTRY/fabric-ca"
  export CA_VERSION=1.5.15

  export MSP_ORG=INMETROMSP
  export PEER_SECRET=peerpw
  export STORAGE_CLASS=local-path
  export DATABASE=leveldb 

  export K8S_BUILDER=false

  # Organização INMETRO
  HLF_OP_DIR="$FABRIC_DIR/fabric-offline/charts/hlf-operator"

    # --namespace hlf-operator --create-namespace --set installCRDs=true \
    # --set kube-rbac-proxy.enabled=true --set kube-rbac-proxy.repository="quay.io/brancz/kube-rbac-proxy" \
    # --set kube-rbac-proxy.tag="v0.14.1" --wait --timeout=120s

  kubectl hlf ca create  --image=$CA_IMAGE --version=$CA_VERSION --storage-class=$STORAGE_CLASS --capacity=1Gi --name=inmetro-ca \
      --enroll-id=enroll --enroll-pw=enrollpw --istio-port=443 --istio-ingressgateway=ingressgateway --hosts="inmetro-ca.inmetro.br" #--output=true > resources/inmetroca.yaml

  sleep 3

  # kubectl apply -f resources/inmetroca.yaml

  kubectl wait --timeout=180s --for=condition=Running fabriccas.hlf.kungfusoftware.es --all

  sleep 3

  kubectl hlf ca register --name=inmetro-ca --user=peer --secret=peerpw \
      --type=peer --enroll-id enroll --enroll-secret=enrollpw --mspid=INMETROMSP --ca-url="https://inmetro-ca.inmetro.br:443"

  sleep 5

  kubectl hlf peer create --statedb=$DATABASE --image=$PEER_IMAGE --version=$PEER_VERSION --storage-class=$STORAGE_CLASS --enroll-id=peer --mspid=$MSP_ORG \
  --enroll-pw=$PEER_SECRET --capacity=5Gi --name=inmetro-peer0 --ca-name=inmetro-ca.default --hosts=peer0.inmetro.br --istio-port=443 --istio-ingressgateway=ingressgateway \

  sleep 5

  kubectl wait --timeout=180s --for=condition=Running fabricpeers.hlf.kungfusoftware.es --all

  echo "Criando 3 orderers"

  # register orderer user
  kubectl hlf ca create  --image=$CA_IMAGE --version=$CA_VERSION --storage-class=$STORAGE_CLASS --capacity=1Gi --name=ord-ca \
      --enroll-id=enroll --enroll-pw=enrollpw --hosts=ord-ca.inmetro.br --istio-ingressgateway=ingressgateway --istio-port=443

  sleep 4

  kubectl wait --timeout=180s --for=condition=Running fabriccas.hlf.kungfusoftware.es --all

  kubectl hlf ca register --name=ord-ca --user=orderer --secret=ordererpw \
      --type=orderer --enroll-id enroll --enroll-secret=enrollpw --mspid=OrdererMSP --ca-url="https://ord-ca.inmetro.br:443"
  sleep 2
  kubectl hlf ordnode create --image=$ORDERER_IMAGE --version=$ORDERER_VERSION \
      --storage-class=$STORAGE_CLASS --enroll-id=orderer --mspid=OrdererMSP \
      --enroll-pw=ordererpw --capacity=2Gi --name=ord-node0 --ca-name=ord-ca.default \
      --hosts=orderer0-ord.inmetro.br --istio-ingressgateway=ingressgateway --istio-port=443 --admin-hosts=admin-orderer0-ord.inmetro.br 
  kubectl hlf ordnode create --image=$ORDERER_IMAGE --version=$ORDERER_VERSION \
      --storage-class=$STORAGE_CLASS --enroll-id=orderer --mspid=OrdererMSP \
      --enroll-pw=ordererpw --capacity=2Gi --name=ord-node1 --ca-name=ord-ca.default \
      --hosts=orderer1-ord.inmetro.br --istio-ingressgateway=ingressgateway --istio-port=443 --admin-hosts=admin-orderer1-ord.inmetro.br
  kubectl hlf ordnode create --image=$ORDERER_IMAGE --version=$ORDERER_VERSION \
      --storage-class=$STORAGE_CLASS --enroll-id=orderer --mspid=OrdererMSP \
      --enroll-pw=ordererpw --capacity=2Gi --name=ord-node2 --ca-name=ord-ca.default \
      --hosts=orderer2-ord.inmetro.br --istio-ingressgateway=ingressgateway --istio-port=443 --admin-hosts=admin-orderer2-ord.inmetro.br

  kubectl wait --timeout=180s --for=condition=Running fabricorderernodes.hlf.kungfusoftware.es --all

  sleep 4
  ## register OrdererMSP Identity
  kubectl hlf ca register --name=ord-ca --user=admin --secret=adminpw \
      --type=admin --enroll-id enroll --enroll-secret=enrollpw --mspid=OrdererMSP #--ca-url="https://ord-ca.inmetro.br:443"

  kubectl hlf identity create --name orderer-admin-sign --namespace default \
      --ca-name ord-ca --ca-namespace default \
      --ca ca --mspid OrdererMSP --enroll-id admin --enroll-secret adminpw # sign identity

  sleep 3

  kubectl hlf identity create --name orderer-admin-tls --namespace default \
      --ca-name ord-ca --ca-namespace default \
      --ca tlsca --mspid OrdererMSP --enroll-id admin --enroll-secret adminpw # tls identity

  sleep 3
  ## register INMETROMSP Identity
  # register
  kubectl hlf ca register --name=inmetro-ca --namespace=default --user=admin --secret=adminpw \
      --type=admin --enroll-id enroll --enroll-secret=enrollpw --mspid=INMETROMSP

  # enroll
  kubectl hlf identity create --name inmetro-admin --namespace default \
      --ca-name inmetro-ca --ca-namespace default \
      --ca ca --mspid INMETROMSP --enroll-id admin --enroll-secret adminpw

  sleep 2

  # criação de canal
  export PEER_ORG_SIGN_CERT=$(kubectl get fabriccas inmetro-ca -o=jsonpath='{.status.ca_cert}')
  export PEER_ORG_TLS_CERT=$(kubectl get fabriccas inmetro-ca -o=jsonpath='{.status.tlsca_cert}')
  export IDENT_8=$(printf "%8s" "")
  export ORDERER_TLS_CERT=$(kubectl get fabriccas ord-ca -o=jsonpath='{.status.tlsca_cert}' | sed -e "s/^/${IDENT_8}/" )
  export ORDERER0_TLS_CERT=$(kubectl get fabricorderernodes ord-node0 -o=jsonpath='{.status.tlsCert}' | sed -e "s/^/${IDENT_8}/" )
  export ORDERER1_TLS_CERT=$(kubectl get fabricorderernodes ord-node1 -o=jsonpath='{.status.tlsCert}' | sed -e "s/^/${IDENT_8}/" )
  export ORDERER2_TLS_CERT=$(kubectl get fabricorderernodes ord-node2 -o=jsonpath='{.status.tlsCert}' | sed -e "s/^/${IDENT_8}/" )

  kubectl apply -f - <<EOF
apiVersion: hlf.kungfusoftware.es/v1alpha1
kind: FabricMainChannel
metadata:
  name: demo
spec:
  name: demo
  adminOrdererOrganizations:
    - mspID: OrdererMSP
  adminPeerOrganizations:
    - mspID: INMETROMSP
  channelConfig:
    application:
      acls: null
      capabilities:
        - V2_0
        - V2_5
      policies: null
    capabilities:
      - V2_0
    orderer:
      batchSize:
        absoluteMaxBytes: 1048576
        maxMessageCount: 10
        preferredMaxBytes: 524288
      batchTimeout: 2s
      capabilities:
        - V2_0
      etcdRaft:
        options:
          electionTick: 10
          heartbeatTick: 1
          maxInflightBlocks: 5
          snapshotIntervalSize: 16777216
          tickInterval: 500ms
      ordererType: etcdraft
      policies: null
      state: STATE_NORMAL
    policies: null
  externalOrdererOrganizations: []
  peerOrganizations:
    - mspID: INMETROMSP
      caName: "inmetro-ca"
      caNamespace: "default"
  identities:
    OrdererMSP:
      secretKey: user.yaml
      secretName: orderer-admin-tls
      secretNamespace: default
    OrdererMSP-sign:
      secretKey: user.yaml
      secretName: orderer-admin-sign
      secretNamespace: default
    INMETROMSP:
      secretKey: user.yaml
      secretName: inmetro-admin
      secretNamespace: default
  externalPeerOrganizations: []
  ordererOrganizations:
    - caName: "ord-ca"
      caNamespace: "default"
      externalOrderersToJoin:
        - host: ord-node0
          port: 7053
        - host: ord-node1
          port: 7053
        - host: ord-node2
          port: 7053
      mspID: OrdererMSP
      ordererEndpoints:
        - orderer0-ord.inmetro.br:443
        - orderer1-ord.inmetro.br:443
        - orderer2-ord.inmetro.br:443
      orderersToJoin: []
  orderers:
    - host: orderer0-ord.inmetro.br
      port: 443
      tlsCert: |-
${ORDERER0_TLS_CERT}
    - host: orderer1-ord.inmetro.br
      port: 443
      tlsCert: |-
${ORDERER1_TLS_CERT}
    - host: orderer2-ord.inmetro.br
      port: 443
      tlsCert: |-
${ORDERER2_TLS_CERT}
EOF

  echo "Aguardando 30 segundos para o canal ser carregado"
  sleep 15
  echo "Ingressando peers da organizaçao INMETRO no canal"


  export IDENT_8=$(printf "%8s" "")
  export ORDERER0_TLS_CERT=$(kubectl get fabricorderernodes ord-node0 -o=jsonpath='{.status.tlsCert}' | sed -e "s/^/${IDENT_8}/" )

  kubectl apply -f - <<EOF
apiVersion: hlf.kungfusoftware.es/v1alpha1
kind: FabricFollowerChannel
metadata:
  name: demo-inmetromsp
spec:
  anchorPeers:
    - host: peer0.inmetro.br
      port: 443 
  hlfIdentity:
    secretKey: user.yaml
    secretName: inmetro-admin
    secretNamespace: default
  mspId: INMETROMSP
  name: demo
  externalPeersToJoin: []
  orderers:
    - certificate: |
${ORDERER0_TLS_CERT}
      url: grpcs://orderer0-ord.inmetro.br:443
  peersToJoin:
    - name: inmetro-peer0
      namespace: default
EOF

  sleep 5
  echo "Criando NetworkConfig e InmetroCP"
  kubectl hlf identity create --name inmetro-admin --namespace default \
      --ca-name inmetro-ca --ca-namespace default \
      --ca ca --mspid INMETROMSP --enroll-id explorer-admin --enroll-secret explorer-adminpw \
      --ca-enroll-id=enroll --ca-enroll-secret=enrollpw --ca-type=admin

  kubectl hlf networkconfig create --name=inmetro-cp \
      -o INMETROMSP -o OrdererMSP -c demo \
      --identities=inmetro-admin.default --secret=inmetro-cp
  echo ""
  sleep 2
  kubectl get pods
  echo "Rede levantada com sucesso!"
}

: '
Derruba a rede mantendo as dependências recicláveis de pé, agilizando o processo de levantamento e economizando recursos. Não deve
ser usado em caso de erro, pois alguns erros na criação do cluster podem ser mantidos e vão continuar com erro.
'

function down() {
  echo "removendo objetos hlf"
  kubectl delete fabricorderernodes.hlf.kungfusoftware.es --all-namespaces --all
  kubectl delete fabricpeers.hlf.kungfusoftware.es --all-namespaces --all
  kubectl delete fabriccas.hlf.kungfusoftware.es --all-namespaces --all
  kubectl delete fabricchaincode.hlf.kungfusoftware.es --all-namespaces --all
  kubectl delete fabricmainchannels --all-namespaces --all
  kubectl delete fabricfollowerchannels --all-namespaces --all
  kubectl delete fabricnetworkconfigs --all-namespaces --all
  kubectl delete fabricidentities --all-namespaces --all

  echo "Rede derrubada. Alguns itens foram mantidos, use "shutdown" ou "frestart" para"
  echo "derrubar por completo (frestart levanta a rede novamente)."
}

: '
Derruba a rede por completo, removendo completamente o cluster e zerando a rede. Recomendado usar em caso de erro, para fazer
um levantamento limpo e livre de erros. 
'

function shutdown() {
  echo "removendo objetos hlf"
  kubectl delete fabricorderernodes.hlf.kungfusoftware.es --all-namespaces --all
  kubectl delete fabricpeers.hlf.kungfusoftware.es --all-namespaces --all
  kubectl delete fabriccas.hlf.kungfusoftware.es --all-namespaces --all
  kubectl delete fabricchaincode.hlf.kungfusoftware.es --all-namespaces --all
  kubectl delete fabricmainchannels --all-namespaces --all
  kubectl delete fabricfollowerchannels --all-namespaces --all
  kubectl delete fabricnetworkconfigs --all-namespaces --all
  kubectl delete fabricidentities --all-namespaces --all
  
  echo "removendo configurações do operador hlf"
  kubectl get crds | grep 'hlf.kungfusoftware.es' | awk '{print $1}' | xargs -r kubectl delete crd 2>/dev/null

  if kubectl get ns hlf-operator >/dev/null 2>&1; then
    echo "Removendo instalação prévia do HLF Operator..."
    helm uninstall hlf-operator -n hlf-operator 2>/dev/null
    kubectl delete namespace hlf-operator --wait=true #2>/dev/null
    sleep 5
  fi
  
  kubectl delete clusterrole hlf-operator-manager-role --ignore-not-found
  kubectl delete clusterrole hlf-operator-proxy-role --ignore-not-found
  kubectl delete clusterrole hlf-operator-metrics-reader --ignore-not-found
  
  kubectl delete clusterrolebinding hlf-operator-manager-rolebinding --ignore-not-found
  kubectl delete clusterrolebinding hlf-operator-proxy-rolebinding --ignore-not-found
  kubectl delete clusterrolebinding hlf-operator-metrics-reader-binding --ignore-not-found

  kubectl delete namespace hlf-operator --ignore-not-found
  kubectl delete deployment hlf-operator-controller-manager -n default
  helm uninstall hlf-operator -n hlf-operator 2>/dev/null

  kubectl delete namespace istio-system --wait=true 2>/dev/null
  kubectl get crds | grep 'istio' | awk '{print $1}' | xargs -r kubectl delete crd 2>/dev/null
  kubectl delete all --all -n istio-system --ignore-not-found

  echo "removendo registry docker"
  docker rm -f registry 2>/dev/null || echo "Registry não estava em execução."

  echo "removendo imagens docker"
  docker rmi localhost:5000/fabric-peer:3.1.0 -f
  docker rmi localhost:5000/fabric-orderer:3.1.0 -f
  docker rmi localhost:5000/fabric-ca:1.5.15 -f

  echo "derrubando imagens k3s"
  /usr/local/bin/k3s-killall.sh 2>/dev/null
  
  echo "processo de derrubamento completo finalizado!"
  echo ""
}

function ccas() {
  CHAINCODE_NAME=""
  LOCAL_MODE=false
  LOCAL_PATH=""
  USER=""
  REGISTRY="localhost:5000"

  if [ $# -eq 0 ]; then
      echo "Uso: inmetro ccas <nome_do_chaincode> [--local <diretorio>] [<usuario_docker>]"
      return 1
  fi

  while [[ $# -gt 0 ]]; do
      arg="$2"

      case "$arg" in
          --local=*)
              LOCAL_MODE=true
              LOCAL_PATH="${arg#--local=}"
              shift
              ;;
          --local|-local|-l)
              LOCAL_MODE=true
              shift
              if [[ -z "$2" || "$2" =~ ^- ]]; then
                  echo "Erro: --local requer um caminho de diretório."
                  return 1
              fi
              LOCAL_PATH="$2"
              shift
              ;;
          --help|-h)
              echo "Uso: inmetro ccas <nome_do_chaincode> [--local <diretorio>] [<usuario_docker>]"
              echo "Opções:"
              echo "  --local <dir>      Usar chaincode local no diretório <dir> (constroi imagem Docker)"
              echo "  -l, -local         Sinônimo de --local"
              return 0
              ;;
          -*)
              echo "Aviso: flag desconhecida '$arg' ignorada."
              shift
              ;;
          *)
              if [[ -z "$CHAINCODE_NAME" ]]; then
                CHAINCODE_NAME="$arg"
              elif [[ -z "$USER" ]]; then
                USER="$arg"
              fi
              shift
              ;;
      esac
  done

  # VALIDAÇÕES
  if [[ -z "$CHAINCODE_NAME" ]]; then
      echo "Erro: nome do chaincode não informado."
      echo "Uso: inmetro ccas <nome_do_chaincode> [--local <diretorio>] [<usuario_docker>]"
      return 1
  fi

  if [[ "$LOCAL_MODE" = true ]]; then
      if [[ -z "$LOCAL_PATH" ]]; then
          echo "Erro: modo local ativado mas nenhum diretório especificado."
          return 1
      fi
      if [[ ! -d "$LOCAL_PATH" ]]; then
          echo "Erro: diretório local '$LOCAL_PATH' não encontrado."
          return 1
      fi
      USER="$REGISTRY"
  else
      if [[ -z "$USER" ]]; then
          echo "Erro: usuário Docker deve ser informado."
          return 1
      fi
  fi

  echo ""
  echo "Chaincode: $CHAINCODE_NAME"
  echo "Modo local: $LOCAL_MODE"
  [[ "$LOCAL_MODE" = true ]] && echo "Diretório local: $LOCAL_PATH"
  [[ "$LOCAL_MODE" = true ]] && echo "Registry: $REGISTRY"
  [[ "$LOCAL_MODE" = false ]] && echo "Usuário Docker: $USER"
  echo ""

  if [ "$LOCAL_MODE" = true ]; then
      echo "Building docker image"
      
      # Verificar se o Dockerfile existe no diretório do chaincode
      if [[ ! -f "$LOCAL_PATH/Dockerfile" ]]; then
          echo "Erro: Dockerfile não encontrado em $LOCAL_PATH"
          echo "Crie um Dockerfile no diretório do chaincode"
          return 1
      fi

      # Nome da imagem no registry local
      IMAGE_NAME="$REGISTRY/$CHAINCODE_NAME:latest"
      
      # Construir a imagem
      echo "Building image $IMAGE_NAME from $LOCAL_PATH..."
      if ! docker build -t "$IMAGE_NAME" "$LOCAL_PATH"; then
          echo "Erro ao construir imagem Docker"
          return 1
      fi

      if ! docker push "$IMAGE_NAME"; then
          echo "Erro ao fazer push para registry local"
          return 1
      fi
  fi

  echo "Aguardando 3s"
  sleep 3

  # fetch connection
  mkdir -p "$FABRIC_DIR/resources"
  kubectl get secret inmetro-cp -o jsonpath="{.data.config\.yaml}" | base64 --decode > "$FABRIC_DIR/resources/inmetro.yaml"

  echo "Aguardando 2 segundos"
  sleep 2

  # EMPACOTAMENTO
  rm -f code.tar.gz chaincode.tgz "${CHAINCODE_NAME}.tar.gz" metadata.json connection.json

  cat > "metadata.json" <<METADATA_EOF
{
  "type": "ccaas", 
  "label": "${CHAINCODE_NAME}"
}
METADATA_EOF

  cat > "connection.json" <<CONN_EOF
{
  "address": "${CHAINCODE_NAME}:7052",
  "dial_timeout": "10s",
  "tls_required": false
}
CONN_EOF

  tar cfz code.tar.gz connection.json
  tar cfz chaincode.tgz metadata.json code.tar.gz
  PACKAGE_FILE="chaincode.tgz"

  # Verificar se o pacote foi criado
  if [ ! -f "$PACKAGE_FILE" ]; then
      echo "ERRO: Pacote não foi criado: $PACKAGE_FILE"
      ls -la
      return 1
  fi

  export PACKAGE_ID=$(kubectl hlf chaincode calculatepackageid --path="$PACKAGE_FILE" --language=golang --label="$CHAINCODE_NAME")

  if [ -z "$PACKAGE_ID" ]; then
      echo "ERRO: Não foi possível calcular Package ID"
      return 1
  fi

  # INSTALAR
  echo "Instalando chaincode no peer"
  kubectl hlf chaincode install --path="./$PACKAGE_FILE" \
      --config="$FABRIC_DIR/resources/inmetro.yaml" --language=golang --label="$CHAINCODE_NAME" \
      --user=inmetro-admin-default --peer=inmetro-peer0.default

  if [ $? -ne 0 ]; then
      echo "ERRO: Falha na instalação do chaincode"
      return 1
  fi

  echo "Aguardando 10 segundos para instalação"
  sleep 10
  echo "Sincronizando chaincode no Kubernetes..."
  
  if [ "$LOCAL_MODE" = true ]; then
      IMAGE="$REGISTRY/$CHAINCODE_NAME:latest"
  else
      IMAGE="$USER/$CHAINCODE_NAME:latest"
  fi
  
  kubectl hlf externalchaincode sync --image="$IMAGE" \
      --name="$CHAINCODE_NAME" \
      --namespace=default \
      --package-id="$PACKAGE_ID" \
      --tls-required=false \
      --replicas=1 \
      --env "CORE_CHAINCODE_ID_NAME=$CHAINCODE_NAME"

  if [ $? -ne 0 ]; then
      echo "ERRO: Falha ao sincronizar chaincode externo"
      return 1
  fi

  echo "Aguardando 30 segundos para inicialização do pod do chaincode"
  sleep 30

  # Verificar se o pod foi criado
  echo "Verificando pods do chaincode"
  if kubectl get pods -n default | grep -q "$CHAINCODE_NAME"; then
      kubectl get pods -n default | grep "$CHAINCODE_NAME"
  else
      echo "AVISO: Pod do chaincode não encontrado. Verificando os pods"
      kubectl get pods -n default
  fi

  # APPROVE
  SEQUENCE=1
  VERSION="1.0"
  ENDORSEMENT_POLICY="OR('INMETROMSP.member')"

  echo "Aprovando chaincode"
  kubectl hlf chaincode approveformyorg \
      --config="$FABRIC_DIR/resources/inmetro.yaml" --user=inmetro-admin-default --peer=inmetro-peer0.default \
      --package-id="$PACKAGE_ID" --version "$VERSION" --sequence "$SEQUENCE" \
      --name="$CHAINCODE_NAME" --policy="$ENDORSEMENT_POLICY" --channel=demo

  echo "Aguardando 5 segundos"
  sleep 5

  # COMMIT
  echo "Comitando chaincode"
  kubectl hlf chaincode commit \
      --config="$FABRIC_DIR/resources/inmetro.yaml" --user=inmetro-admin-default --mspid=INMETROMSP \
      --version "$VERSION" --sequence "$SEQUENCE" --name="$CHAINCODE_NAME" \
      --policy="$ENDORSEMENT_POLICY" --channel=demo

  # LIMPEZA
  rm -f code.tar.gz chaincode.tgz metadata.json connection.json "${CHAINCODE_NAME}.tar.gz" 2>/dev/null || true

  echo ""
  echo "Chaincode $CHAINCODE_NAME instalado com sucesso!"
  echo ""
  
  return 0
}

function ccasupgrade() {
    CHAINCODE_NAME=""
    VERSION=""
    SEQUENCE=""
    LOCAL_MODE=false
    LOCAL_PATH=""
    USER=""
    REGISTRY="localhost:5000"

    if [ $# -lt 3 ]; then
        echo "Uso: inmetro ccasupgrade <nome_do_chaincode> <versao> <sequencia> [--local <diretorio>] [<usuario_docker>]"
        return 1
    fi

    CHAINCODE_NAME="$2"
    VERSION="$3"
    SEQUENCE="$4"
    shift 3

    while [[ $# -gt 0 ]]; do
        arg="$2"

        case "$arg" in
            --local=*)
                LOCAL_MODE=true
                LOCAL_PATH="${arg#--local=}"
                shift
                ;;
            --local|-local|-l)
                LOCAL_MODE=true
                shift
                if [[ -z "$2" || "$2" =~ ^- ]]; then
                    echo "Erro: --local requer um caminho de diretório."
                    return 1
                fi
                LOCAL_PATH="$2"
                shift
                ;;
            --help|-h)
                echo "Uso: inmetro ccasupgrade <nome_do_chaincode> <versao> <sequencia> [--local <diretorio>] [<usuario_docker>]"
                echo "Opções:"
                echo "  --local <dir>      Usar chaincode local no diretório <dir> (constroi imagem Docker)"
                echo "  -l, -local         Sinônimo de --local"
                return 0
                ;;
            -*)
                echo "Aviso: flag desconhecida '$arg' ignorada."
                shift
                ;;
            *)
                if [[ -z "$USER" ]]; then
                    USER="$arg"
                    shift
                else
                    shift
                fi
                ;;
        esac
    done

    # VALIDAÇÕES
    if [[ -z "$CHAINCODE_NAME" ]]; then
        echo "Erro: nome do chaincode não informado."
        return 1
    fi

    if [[ -z "$VERSION" ]]; then
        echo "Erro: versão não informada."
        return 1
    fi

    if [[ ! "$VERSION" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
        echo "Erro: versão '$VERSION' deve ser um número."
        return 1
    fi

    if [[ -z "$SEQUENCE" ]]; then
        echo "Erro: sequência não informada."
        return 1
    fi

    # Validar que SEQUENCE é um número inteiro
    if [[ ! "$SEQUENCE" =~ ^[0-9]+$ ]]; then
        echo "Erro: sequência '$SEQUENCE' deve ser um número inteiro."
        return 1
    fi

    if [[ "$LOCAL_MODE" = true ]]; then
        if [[ -z "$LOCAL_PATH" ]]; then
            echo "Erro: modo local ativado mas nenhum diretório especificado."
            return 1
        fi
        if [[ ! -d "$LOCAL_PATH" ]]; then
            echo "Erro: diretório local '$LOCAL_PATH' não encontrado."
            return 1
        fi
        USER="$REGISTRY"
    else
        if [[ -z "$USER" ]]; then
            echo "Erro: usuário Docker deve ser informado."
            return 1
        fi
    fi

    echo ""
    echo "Chaincode: $CHAINCODE_NAME"
    echo "Versão: $VERSION"
    echo "Sequência: $SEQUENCE"
    echo "Modo local: $LOCAL_MODE"
    [[ "$LOCAL_MODE" = true ]] && echo "Diretório local: $LOCAL_PATH"
    [[ "$LOCAL_MODE" = true ]] && echo "Registry: $REGISTRY"
    [[ "$LOCAL_MODE" = false ]] && echo "Usuário Docker: $USER"
    echo ""

    if [ "$LOCAL_MODE" = true ]; then
        if [[ ! -f "$LOCAL_PATH/Dockerfile" ]]; then
            echo "Erro: Dockerfile não encontrado em $LOCAL_PATH"
            echo "Crie um Dockerfile no diretório do chaincode"
            return 1
        fi

        # Nome da imagem no registry local
        IMAGE_NAME="$REGISTRY/$CHAINCODE_NAME:latest"
        
        echo "building updated chaincode image $IMAGE_NAME from $LOCAL_PATH..."
        if ! docker build -t "$IMAGE_NAME" "$LOCAL_PATH"; then
            echo "Erro ao construir imagem Docker"
            return 1
        fi

        if ! docker push "$IMAGE_NAME"; then
            echo "Erro ao fazer push para registry local"
            return 1
        fi
    else
        IMAGE_NAME="$USER/$CHAINCODE_NAME:latest"
        echo "Pulling from docker: $IMAGE_NAME"
    fi

    echo "Aguardando 3s para propagação..."
    sleep 3

    # fetch connection
    mkdir -p "$FABRIC_DIR/resources"
    kubectl get secret inmetro-cp -o jsonpath="{.data.config\.yaml}" | base64 --decode > "$FABRIC_DIR/resources/inmetro.yaml"

    echo "Aguardando 2 segundos..."
    sleep 2

    rm -f code.tar.gz chaincode.tgz metadata.json connection.json
    cat > "metadata.json" <<METADATA_EOF
{
    "type": "ccaas", 
    "label": "${CHAINCODE_NAME}"
}
METADATA_EOF

    cat > "connection.json" <<CONN_EOF
{
    "address": "${CHAINCODE_NAME}:7052",
    "dial_timeout": "10s",
    "tls_required": false
}
CONN_EOF

    tar cfz code.tar.gz connection.json
    tar cfz chaincode.tgz metadata.json code.tar.gz
    PACKAGE_FILE="chaincode.tgz"

    # Verificar se o pacote foi criado
    if [ ! -f "$PACKAGE_FILE" ]; then
        echo "ERRO: Pacote não foi criado: $PACKAGE_FILE"
        return 1
    fi

    # CALCULAR PACKAGE_ID
    export PACKAGE_ID=$(kubectl hlf chaincode calculatepackageid --path="$PACKAGE_FILE" --language=golang --label="$CHAINCODE_NAME")
    echo "PACKAGE_ID=$PACKAGE_ID"

    if [ -z "$PACKAGE_ID" ]; then
        echo "ERRO: Não foi possível calcular Package ID"
        return 1
    fi

    kubectl hlf chaincode install --path="./$PACKAGE_FILE" \
        --config="$FABRIC_DIR/resources/inmetro.yaml" --language=golang --label="$CHAINCODE_NAME" \
        --user=inmetro-admin-default --peer=inmetro-peer0.default

    if [ $? -ne 0 ]; then
        echo "ERRO: Falha na instalação do chaincode"
        return 1
    fi

    echo "Aguardando 10 segundos para instalação..."
    sleep 10

    echo "Sincronizando chaincode atualizado no Kubernetes..."    
    kubectl hlf externalchaincode sync --image="$IMAGE_NAME" \
        --name="$CHAINCODE_NAME" \
        --namespace=default \
        --package-id="$PACKAGE_ID" \
        --tls-required=false \
        --replicas=1 \
        --env "CORE_CHAINCODE_ID_NAME=$CHAINCODE_NAME"

    if [ $? -ne 0 ]; then
        echo "ERRO: Falha ao sincronizar chaincode externo"
        return 1
    fi

    echo "Aguardando 30 segundos para atualização do pod..."
    sleep 30

    echo "Verificando pods do chaincode atualizado..."
    if kubectl get pods -n default | grep -q "$CHAINCODE_NAME"; then
        kubectl get pods -n default | grep "$CHAINCODE_NAME"
    else
        echo "AVISO: Pod do chaincode não encontrado."
    fi

    ENDORSEMENT_POLICY="OR('INMETROMSP.member')"

    echo "Aprovando nova versão do chaincode"
    kubectl hlf chaincode approveformyorg \
        --config="$FABRIC_DIR/resources/inmetro.yaml" --user=inmetro-admin-default --peer=inmetro-peer0.default \
        --package-id="$PACKAGE_ID" --version "$VERSION" --sequence "$SEQUENCE" \
        --name="$CHAINCODE_NAME" --policy="$ENDORSEMENT_POLICY" --channel=demo

    if [ $? -ne 0 ]; then
        echo "ERRO: Falha ao aprovar chaincode"
        return 1
    fi

    echo "Aguardando 5 segundos"
    sleep 5

    # COMMIT
    echo "Comitando nova versão do chaincode"
    kubectl hlf chaincode commit \
        --config="$FABRIC_DIR/resources/inmetro.yaml" --user=inmetro-admin-default --mspid=INMETROMSP \
        --version "$VERSION" --sequence "$SEQUENCE" --name="$CHAINCODE_NAME" \
        --policy="$ENDORSEMENT_POLICY" --channel=demo

    if [ $? -ne 0 ]; then
        echo "ERRO: Falha ao comitar chaincode"
        return 1
    fi

    # LIMPEZA
    rm -f code.tar.gz chaincode.tgz metadata.json connection.json 2>/dev/null || true

    echo ""
    echo "Chaincode $CHAINCODE_NAME atualizado com sucesso!"
    echo "Versão: $VERSION"
    echo "Sequência: $SEQUENCE"    
    return 0
}

CMD_VAL=("up" "lup" "down" "shutdown" "lrestart" "restart" "ccas" "ccasupgrade" "help" "k3s")
cmd_short() {
  local input=$1
  shift
  local cmd_val=("$@")
  
  for cmd in "${cmd_val[@]}"; do
    if [[ "$cmd" == "$input" ]]; then
      echo "$cmd"
      return 0
    fi
  done

  local cmd_pos=()
  for cmd in "${cmd_val[@]}"; do
    if [[ "$cmd" == "$input"* ]]; then
      cmd_pos+=("$cmd")
    fi
  done

  if [ ${#cmd_pos[@]} -eq 0 ]; then
    echo "Comando não encontrado."
    return 1
  fi

  if [ ${#cmd_pos[@]} -gt 1 ]; then
    echo "${cmd_pos[*]}"
    return 1
  fi

  echo "${cmd_pos[0]}"
  return 0
}

CMD_RES=$(cmd_short "$1" "${CMD_VAL[@]}")
if [ $? -ne 0 ]; then
  echo "$CMD_RES"
  exit 1
fi

case "$CMD_RES" in
  up)
    up
    ;;
  lup)
    lup
    ;;
  down)
    down
    ;;
  shutdown)
    shutdown
    ;;
  lrestart)
    down
    sleep 3
    lup
    ;;
  restart)
    shutdown
    sleep 3
    up
    ;;
  ccas)
    ccas "$@"
    ;;
  ccasupgrade)
    ccasupgrade "$@"
    ;;
  k3s)
    k3s
    ;;
  help)
    echo "Hyperledger Fabric - Inmetro"
    echo ""
    echo "Essa é uma versão da rede em desenvolvimento. Alguns bugs ainda podem acontecer. Caso aconteça, reporte!"
    echo ""
    echo "Comandos disponíveis: "
    echo "  up - levanta a rede de forma offline."
    echo "  lup - levanta a rede reciclando componentes (não funciona como primeiro levantamento, o comando 'up' deve ser usado antes." 
    echo "  down - derruba a rede mas mantém alguns componentes que podem continuar de pé."
    echo "  shutdown - derruba a rede e todas as suas dependências (depende do up para levantar novamente)."
    echo "  lrestart - reinicia a rede mantendo alguns componentes."
    echo "  restart - reinicia a rede por completo (use para corrigir erros)."
    echo "  ccas - instala um chaincode usando Chaincode as a Service. Pode instalar de forma local ou usando imagem docker online."
    echo "         Use o comando 'inmetro ccas -h' para mais informações."
    echo "  ccasupgrade - atualiza um chaincode já instalado. Precisa de versão, sequência e do username docker como parâmetros."
    echo "                Use o comando 'inmetro ccasupgrade -h' para mais informações."
    echo "  k3s - reinstala o k3s completamente, usado apenas para resolver bugs no desenvolvimento de chaincodes."
    echo ""
    echo "OBS: Se a rede foi iniciada anteriormente com acesso à internet e depois derrubada com o comando que recicla os recursos, tentar reiniciá-la em um ambiente com uma configuração de conexão diferente (com ou sem internet) pode causar erros."
    ;;
  *)
    echo "Comando não existe."
    ;;
esac
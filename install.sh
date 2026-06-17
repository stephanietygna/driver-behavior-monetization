#!/bin/bash

set -e

FABRIC_DIR="$(cd "$(dirname "$0")" && pwd)"
mkdir -p $FABRIC_DIR/fabric-offline/bin
echo "$FABRIC_DIR" | sudo tee /etc/fabric_dir.conf > /dev/null  
cd $FABRIC_DIR/fabric-offline
echo $FABRIC_DIR

echo "Atualizando pacotes"
sudo apt update && sudo apt upgrade -y
sudo apt install -y curl jq gpg apt-transport-https tar

#Instalação dinamica do kubectl (usa uma versão fixa caso dê erro com snap)
if sudo snap install kubectl --classic 2>/dev/null; then
    echo "kubectl instalado via SNAP"
else
    echo "Snap falhou, tentando método alternativo..."
    
    KUBECTL_VERSION=$(curl -L -s https://dl.k8s.io/release/stable.txt)
    echo "Baixando kubectl versão $KUBECTL_VERSION"
    
    curl -LO "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/arm64/kubectl"
    
    if [ -f "kubectl" ]; then
        # Instalar no sistema
        chmod +x kubectl
        sudo mv kubectl /usr/local/bin/
        echo "kubectl instalado"
    else
        echo "Falha no download do kubectl"
        exit 1
    fi
fi

# instalando k3s
# echo "Configurando interface dummy para k3s"
# if ! ip link show tap0 &> /dev/null; then
#   echo 'configurando interface dummy para k3s'
#   sudo ip tuntap add mode tap tap0
#   sudo ip addr add 10.243.255.254/32 dev tap0
#   sudo ip link set tap0 up
# fi

echo "Instalando k3s"
sudo curl -k -sfL https://get.k3s.io | sh -s - --disable traefik --write-kubeconfig-mode 644 --node-name k3s-master-01
    
sudo systemctl daemon-reload
sudo systemctl restart k3s

# Instalar Krew
echo "Instalando Krew"
(
  set -x; cd "$(mktemp -d)" &&
  OS="$(uname | tr '[:upper:]' '[:lower:]')" &&
  ARCH="$(uname -m | sed -e 's/x86_64/amd64/' -e 's/\(arm\)\(64\)\?.*/\1\2/' -e 's/aarch64$/arm64/')" &&
  KREW="krew-${OS}_${ARCH}" &&
  curl -fsSLO "https://github.com/kubernetes-sigs/krew/releases/latest/download/${KREW}.tar.gz" &&
  tar zxvf "${KREW}.tar.gz" &&
  ./"${KREW}" install krew
)

if ! grep -q "${KREW_ROOT:-$HOME/.krew}/bin" "$HOME/.bashrc"; then
    echo "export PATH=\"${KREW_ROOT:-$HOME/.krew}/bin:\$PATH\"" >> "$HOME/.bashrc"
fi

export PATH="${KREW_ROOT:-$HOME/.krew}/bin:$PATH"

# instalando hlf via krew
kubectl krew install hlf

# Instalar Helm
# curl https://baltocdn.com/helm/signing.asc | gpg --dearmor | sudo tee /usr/share/keyrings/helm.gpg > /dev/null
# sudo apt-get install apt-transport-https --yes
# echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/helm.gpg] https://baltocdn.com/helm/stable/debian/ all main" | sudo tee /etc/apt/sources.list.d/helm-stable-debian.list
# sudo apt update
# sudo apt install helm
# sudo snap install helm --classic

curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3
chmod 700 get_helm.sh
./get_helm.sh

# Instalar docker
echo "Instalando docker..."
if sudo snap install docker --classic 2>/dev/null; then
    echo "docker instalado via snap"
else
    echo "Instalando docker..."
    curl -fsSL https://get.docker.com -o get-docker.sh
    sh get-docker.sh
    sudo usermod -aG docker $USER
fi

is_docker=0

if ! getent group docker > /dev/null; then
  echo "Criando o grupo 'docker'..."
  sudo groupadd docker
fi

if id -nG "$USER" | grep -qw docker; then
  if docker run --rm alpine echo "Docker OK" &> /dev/null; then
      is_docker=1
  else
      is_docker=0
  fi
else
  sudo usermod -aG docker "$USER"
  is_docker=0
fi

# Instalar Istioctl (mudar a versão pra 1.25 ou 1.26)
if ! command -v istioctl &>/dev/null; then
  echo "Istio não encontrado. Instalando..."
  curl -L https://istio.io/downloadIstio | ISTIO_VERSION=1.26.0 TARGET_ARCH=x86_64 sh -
  mv istio-1.26.0/ istioctl/

  if ! grep -q "$FABRIC_DIR/fabric-offline/istioctl/bin" "$HOME/.bashrc"; then
    echo "export PATH=\"$FABRIC_DIR/fabric-offline/istioctl/bin:\$PATH\"" >> "$HOME/.bashrc"
  fi

  rm -rf istio-1.26.0
  echo "Istio foi instalado com sucesso!"
fi

sleep 5

# Baixando operador HLF
echo "Baixando HLF Operator"
CHARTS_DIR="$FABRIC_DIR/fabric-offline/hlf-operator"

if [ ! -d "$CHARTS_DIR" ]; then
  mkdir -p "$CHARTS_DIR"
  helm repo add kfs https://kfsoftware.github.io/hlf-helm-charts --force-update 
  helm repo update
  helm pull kfs/hlf-operator --version=1.13.0
  mv hlf-operator-1.13.0.tgz ./hlf-operator/
fi

if ! grep -q "$FABRIC_DIR/fabric-offline/bin" "$HOME/.bashrc"; then
    echo "export PATH=\"$FABRIC_DIR/fabric-offline/bin:\$PATH\"" >> "$HOME/.bashrc"
fi
source ~/.bashrc

# Baixando imagens localmente
echo "Criando diretório de imagens"
IMG_DIR=$FABRIC_DIR/fabric-offline/images

# Finalização
sudo cp $PWD/../docs/loadbalancer.sh /usr/local/bin/inmetro
sudo chmod 755 /usr/local/bin/inmetro
mkdir -p "$IMG_DIR"

if [ "$is_docker" == 0 ]; then
  echo "Baixando imagens"
  sudo docker pull hyperledger/fabric-peer:3.1.0
  sudo docker pull hyperledger/fabric-orderer:3.1.0
  sudo docker pull hyperledger/fabric-ca:1.5.15
  sudo docker pull quay.io/brancz/kube-rbac-proxy:v0.14.1
  sudo docker pull docker.io/istio/pilot:1.26.0
  sudo docker pull docker.io/istio/proxyv2:1.26.0

  echo "Salvando imagens em diretório temporário"
  TMP_DIR="~/tmp/docker_images"
  sudo mkdir -p "$TMP_DIR"

  sudo docker save hyperledger/fabric-peer:3.1.0 -o "$TMP_DIR/fabric-peer-3.1.0.tar"
  sudo docker save hyperledger/fabric-orderer:3.1.0 -o "$TMP_DIR/fabric-orderer-3.1.0.tar"
  sudo docker save hyperledger/fabric-ca:1.5.15 -o "$TMP_DIR/fabric-ca-1.5.15.tar"
  sudo docker save quay.io/brancz/kube-rbac-proxy:v0.14.1 -o "$TMP_DIR/kube-rbac-proxy-v0.14.1.tar"
  sudo docker save docker.io/istio/pilot:1.26.0 -o "$TMP_DIR/istio-pilot-1.26.0.tar"
  sudo docker save docker.io/istio/proxyv2:1.26.0 -o "$TMP_DIR/istio-proxyv2-1.26.0.tar"
  
  echo "Movendo para o diretório offline"
  sudo mv "$TMP_DIR"/*.tar "$IMG_DIR"
  sudo chown -R "$USER:$USER" "$IMG_DIR"
  chmod -R u+rwX "$IMG_DIR"
  sudo rm -rf "$TMP_DIR"

  echo "Imagens salvas com sucesso."

  echo "Instalação concluída. Algumas alterações requerem a reinicialização do sistema."
  read -p "Deseja reiniciar agora? (s/n): " resposta
  resposta=${resposta,,}

  if [[ "$resposta" =~ ^(s|sim|y|yes)$ ]]; then
    echo "Reiniciando o sistema..."
    sudo reboot

  else
    echo "Reinicialização cancelada. Reinicie quando puder para aplicar as alterações."
  fi

else
  echo "Baixando imagens"
  docker pull hyperledger/fabric-peer:3.1.0
  docker pull hyperledger/fabric-orderer:3.1.0
  docker pull hyperledger/fabric-ca:1.5.15
  # docker pull quay.io/brancz/kube-rbac-proxy:v0.14.1
  docker pull docker.io/istio/pilot:1.26.0
  docker pull docker.io/istio/proxyv2:1.26.0

  echo "Salvando imagens"
  docker save hyperledger/fabric-peer:3.1.0 -o "$IMG_DIR/fabric-peer-3.1.0.tar"
  docker save hyperledger/fabric-orderer:3.1.0 -o "$IMG_DIR/fabric-orderer-3.1.0.tar"
  docker save hyperledger/fabric-ca:1.5.15 -o "$IMG_DIR/fabric-ca-1.5.15.tar"
  # docker save quay.io/brancz/kube-rbac-proxy:v0.14.1 -o "$IMG_DIR/kube-rbac-proxy-v0.14.1.tar"
  docker save docker.io/istio/pilot:1.26.0 -o "$IMG_DIR/istio-pilot-1.26.0.tar"
  docker save docker.io/istio/proxyv2:1.26.0 -o "$IMG_DIR/istio-proxyv2-1.26.0.tar"
  
  sudo chown -R "$USER:$USER" "$IMG_DIR"
  chmod -R u+rwX "$IMG_DIR"
  
  echo "Imagens salvas com sucesso."

  echo "Instalação concluída"
  
  echo ""
fi
### Comandos úteis para depuração / debug

- kubectl get pods
- kubectl get fabricmainchannel -A
- kubectl get fabricfollowerchannel <channel> -o yaml
- kubectl get fabricchaincodes  -A
- kubectl get fabriccas.hlf.kungfusoftware.es -o yaml
- kubectl logs -c manager -f <operador>
- kubectl logs <podname>
- kubectl describe pod <podname>
- docker ps

### Verificação de RAM

- free --human
- htop

### Conexão SSH

- sudo arping <ip>
- openssl s_client -connect <domain>
- curl -k <domain>
- ssh -f victor@192.168.56.108 -L 38325:172.18.0.2:6443 -N (MUDE O IP E PORTA DEPENDENDO DA MAQUINA VIRTUAL)


### Apagar canais
- kubectl delete fabricmainchannel <channel>
- kubectl delete fabricfollowerchannel <channel>


### Conexão remota via Lens
//(trocar no 2º IP pela porta no kubeconfig)
ssh -f inmetro@192.168.56.104 -L 6443:127.0.0.1:6443 -N 

# EM CASO DE ERRO -> https://docs.k3s.io/cluster-access
# Funções de chaincode devem começar com letra maíuscula

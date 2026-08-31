import time
import loghill


# Inicialize antes de importar frameworks ou módulos que criem handlers próprios.
log = loghill.instrument()


def simular_traceback():
    dados = {"pedido": "BB-123", "status": "pendente"}
    return processar_pedido(dados)


def processar_pedido(dados):
    raise RuntimeError(f"Falha proposital no processamento do pedido {dados['pedido']}")


def main():
    time.sleep(1)

    while True:
        print("""
1. INFO Iniciando a aplicação...
2. DEBUG Esta é uma mensagem de depuração.
3. WARNING Esta é uma mensagem de aviso.
4. ERROR Esta é uma mensagem de erro.
5. CRITICAL Esta é uma mensagem crítica.

6. INFO Esta é uma mensagem de informação com parâmetros.
7. INFO Esta é uma mensagem de informação com ECONNRESET (RabbitMQ).

8. Simulação de vários logs por 30 segundos.

9. Traceback nao tratado para testar envio como ERROR.
""")
        while True:
            usr = input("Digite um comando (1/2/3/4/5/6/7/8/9): ").strip().lower()

            match usr:
                case "1":
                    log.info("Iniciando a aplicação...")
                case "2":
                    log.debug("Esta é uma mensagem de depuração.")
                case "3":
                    log.warning("Esta é uma mensagem de aviso.")
                case "4":
                    log.error("Esta é uma mensagem de erro.")
                case "5":
                    log.critical("Esta é uma mensagem crítica.")
                case "6":
                    log.info("Esta é uma mensagem de informação com parâmetros.", event=~"")
                case "7":
                    log.info("Esta é uma mensagem de informação com ECONNRESET (RabbitMQ).")
                case "8":
                    print("Iniciando simulação de logs por 30 segundos...")
                    messages = [
                        (log.info, "Simulação: conexão estabelecida com sucesso."),
                        (log.debug, "Simulação: estado interno de depuração."),
                        (log.warning, "Simulação: uso de recurso acima do esperado."),
                        (log.error, "Simulação: erro simulado detectado."),
                        (log.critical, "Simulação: falha crítica simulada."),
                    ]
                    end_time = time.time() + 30
                    idx = 0
                    while time.time() < end_time:
                        fn, message = messages[idx % len(messages)]
                        fn(message)
                        idx += 1
                        time.sleep(1)
                    print("Simulação de logs finalizada.")
                case "9":
                    simular_traceback()
                case _:
                    print("Comando inválido. Tente novamente.")

if __name__ == "__main__":
    main()

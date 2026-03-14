import socket
import json
import threading
import time
import random
import uuid

class TCPTestClient:
    def __init__(self, server_host='localhost', server_port=17890):
        self.server_host = server_host
        self.server_port = server_port
        self.socket = None
        self.connected = False

    def connect(self):
        """连接到服务器"""
        try:
            self.socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            self.socket.connect((self.server_host, self.server_port))
            self.connected = True
            print(f"已连接到服务器 {self.server_host}:{self.server_port}")
            return True
        except Exception as e:
            print(f"连接失败: {e}")
            return False

    def disconnect(self):
        """断开连接"""
        if self.socket:
            self.socket.close()
            self.connected = False
            print("已断开连接")

    def send_message(self, message):
        """发送消息到服务器"""
        if not self.connected:
            print("未连接到服务器")
            return False

        try:
            # 确保消息以换行符结尾
            if not message.endswith('\n'):
                message += '\n'

            self.socket.send(message.encode('utf-8'))
            print(f"发送: {message.strip()}")
            return True
        except Exception as e:
            print(f"发送失败: {e}")
            return False

    def receive_message(self, timeout=5):
        """接收服务器响应"""
        if not self.connected:
            print("未连接到服务器")
            return None

        try:
            self.socket.settimeout(timeout)
            response = self.socket.recv(4096).decode('utf-8').strip()
            print(f"接收: {response}")
            return response
        except socket.timeout:
            print("接收超时")
            return None
        except Exception as e:
            print(f"接收失败: {e}")
            return None

    def authenticate(self, user_id=None, token="test_token"):
        """执行认证"""
        if user_id is None:
            user_id = str(uuid.uuid4())

        auth_request = {
            "op": "auth",
            "payload": {
                "id": user_id,
                "token": token
            }
        }

        self.send_message(json.dumps(auth_request))
        response = self.receive_message()
        return response

    def send_heartbeat(self, version=[1, 0, 0]):
        """发送心跳包"""
        heartbeat_request = {
            "op": "Beat",
            "payload": {
                "version": version,
                "timestamp": int(time.time() * 1000),
                "status": "active",
                "delay": 0
            }
        }

        self.send_message(json.dumps(heartbeat_request))
        response = self.receive_message()
        return response

    def send_ping(self):
        """发送ping请求"""
        ping_request = {
            "op": "Ping",
            "payload": {
                "version": [1, 0, 0],
                "timestamp": int(time.time() * 1000),
                "status": "active",
                "delay": 0
            }
        }

        self.send_message(json.dumps(ping_request))
        response = self.receive_message()
        return response

    def send_message_to_user(self, to_uuid, content, sender_uuid):
        """发送消息到指定用户"""
        message_request = {
            "op": "Message",
            "payload": {
                "id": str(uuid.uuid4()),
                "form": sender_uuid,  # 发送者的UUID
                "to": to_uuid,        # 接收者的UUID
                "type": "Text",
                "content": content,
                "timestamp": int(time.time() * 1000)
            }
        }

        self.send_message(json.dumps(message_request))
        response = self.receive_message()
        return response

def test_authentication_required():
    """测试必须先认证的功能"""
    print("\n=== 测试未认证访问被拒绝 ===")
    client = TCPTestClient()

    if client.connect():
        # 尝试直接发送心跳而不认证 - 应该被拒绝
        print("\n尝试发送心跳而不先认证...")
        client.send_heartbeat()
        response = client.receive_message()

        if response and '"code":"0010"' in response:
            print("✓ 服务器正确拒绝了未认证的请求")
        else:
            print("✗ 服务器未正确拒绝未认证的请求")

        # 现在进行认证
        print("\n进行认证...")
        auth_response = client.authenticate()

        if auth_response and '"code":"0000"' in auth_response:
            print("✓ 认证成功")

            # 认证后再发送心跳 - 应该成功
            print("\n认证后发送心跳...")
            heartbeat_response = client.send_heartbeat()
        else:
            print("✗ 认证失败")

        client.disconnect()

def test_normal_flow():
    """测试正常的认证和通信流程"""
    print("\n=== 测试正常认证和通信流程 ===")
    client = TCPTestClient()

    if client.connect():
        # 认证
        print("\n进行认证...")
        auth_response = client.authenticate()

        if auth_response and '"code":"0000"' in auth_response:
            print("✓ 认证成功")

            # 发送心跳
            print("\n发送心跳...")
            client.send_heartbeat()

            # 发送ping
            print("\n发送ping...")
            client.send_ping()
        else:
            print("✗ 认证失败")

        client.disconnect()

def test_multiple_clients():
    """测试多个客户端"""
    print("\n=== 测试多客户端场景 ===")

    def client_worker(client_id, delay=0):
        time.sleep(delay)  # 不同客户端稍有不同的启动时间
        client = TCPTestClient()

        print(f"\n客户端 {client_id} 开始连接...")
        if client.connect():
            # 每个客户端使用不同的UUID
            user_uuid = f"user_{client_id}_{str(uuid.uuid4())[:8]}"
            print(f"客户端 {client_id} 使用UUID: {user_uuid}")

            # 认证
            auth_response = client.authenticate(user_uuid)
            if auth_response and '"code":"0000"' in auth_response:
                print(f"客户端 {client_id} 认证成功")

                # 发送心跳
                client.send_heartbeat()

                # 等待一会儿再断开
                time.sleep(2)
            else:
                print(f"客户端 {client_id} 认证失败")

            client.disconnect()
        else:
            print(f"客户端 {client_id} 连接失败")

    # 启动多个客户端线程
    threads = []
    for i in range(3):
        t = threading.Thread(target=client_worker, args=(i, i*0.5))
        threads.append(t)
        t.start()

    # 等待所有线程完成
    for t in threads:
        t.join()

    print("所有客户端测试完成")

def test_message_forwarding():
    """测试消息转发功能"""
    print("\n=== 测试消息转发功能 ===")

    # 创建两个客户端
    client1 = TCPTestClient()
    client2 = TCPTestClient()

    # 连接第一个客户端并认证
    print("\n连接客户端1...")
    if client1.connect():
        user1_uuid = f"user1_{str(uuid.uuid4())[:8]}"
        print(f"客户端1使用UUID: {user1_uuid}")

        auth_response1 = client1.authenticate(user1_uuid)
        if auth_response1 and '"code":"0000"' in auth_response1:
            print("客户端1认证成功")
        else:
            print("客户端1认证失败")
            client1.disconnect()

    # 连接第二个客户端并认证
    print("\n连接客户端2...")
    if client2.connect():
        user2_uuid = f"user2_{str(uuid.uuid4())[:8]}"
        print(f"客户端2使用UUID: {user2_uuid}")

        auth_response2 = client2.authenticate(user2_uuid)
        if auth_response2 and '"code":"0000"' in auth_response2:
            print("客户端2认证成功")
        else:
            print("客户端2认证失败")
            client2.disconnect()

    # 客户端1向客户端2发送消息
    if client1.connected and client2.connected:
        print(f"\n客户端1向客户端2({user2_uuid})发送消息...")
        message_response = client1.send_message_to_user(user2_uuid, "Hello from Client1!", user1_uuid)
        print(f"客户端1收到响应: {message_response}")

    # 断开连接
    if client1.connected:
        client1.disconnect()
    if client2.connected:
        client2.disconnect()

def interactive_test():
    """交互式测试"""
    print("\n=== 交互式测试 ===")
    client = TCPTestClient()

    while True:
        action = input("\n选择操作 (connect/auth/heartbeat/ping/message/disconnect/quit): ").strip().lower()

        if action == 'connect':
            client.connect()
        elif action == 'auth':
            if client.connected:
                user_id = input("请输入用户ID (留空生成随机UUID): ").strip()
                if not user_id:
                    user_id = str(uuid.uuid4())
                client.authenticate(user_id)
            else:
                print("请先连接服务器")
        elif action == 'heartbeat':
            if client.connected:
                client.send_heartbeat()
            else:
                print("请先连接服务器")
        elif action == 'ping':
            if client.connected:
                client.send_ping()
            else:
                print("请先连接服务器")
        elif action == 'message':
            if client.connected:
                to_uuid = input("请输入目标UUID: ").strip()
                content = input("请输入消息内容: ").strip()
                sender_uuid = input("请输入发送者UUID: ").strip()
                client.send_message_to_user(to_uuid, content, sender_uuid)
            else:
                print("请先连接服务器")
        elif action == 'disconnect':
            client.disconnect()
        elif action == 'quit':
            if client.connected:
                client.disconnect()
            break
        else:
            print("无效操作")

if __name__ == "__main__":
    print("TCP服务器测试客户端")
    print("服务器地址: localhost:17890")

    while True:
        print("\n请选择测试模式:")
        print("1. 测试认证必需功能")
        print("2. 测试正常流程")
        print("3. 测试多客户端")
        print("4. 测试消息转发")
        print("5. 交互式测试")
        print("6. 运行所有测试")
        print("0. 退出")

        choice = input("请输入选择 (0-6): ").strip()

        if choice == '1':
            test_authentication_required()
        elif choice == '2':
            test_normal_flow()
        elif choice == '3':
            test_multiple_clients()
        elif choice == '4':
            test_message_forwarding()
        elif choice == '5':
            interactive_test()
        elif choice == '6':
            test_authentication_required()
            test_normal_flow()
            test_multiple_clients()
            test_message_forwarding()
            print("\n所有测试完成！")
        elif choice == '0':
            print("退出测试")
            break
        else:
            print("无效选择")
import io
import os
import signal
import sys
import unittest
from unittest import mock

from examples.project_example.worker.core import loghill_runner


class FakeLogger:
    def __init__(self):
        self.entries = []
        self.flushed = False
        self.closed = False

    def send(self, message, *, severity, metadata):
        self.entries.append((message, severity, metadata))

    def flush(self):
        self.flushed = True
        return True

    def close(self):
        self.closed = True


class FakeChild:
    def __init__(self):
        self.pid = 4321
        self.stdout = io.StringIO("saída do filho\n")
        self.stderr = io.StringIO("erro do filho\n")
        self.return_code = None
        self.terminated = False

    def wait(self, timeout=None):
        self.return_code = 0
        return 0

    def poll(self):
        return self.return_code

    def terminate(self):
        self.terminated = True
        self.return_code = 143


class LogHillSupervisorExampleTests(unittest.TestCase):
    def test_child_command_uses_current_python_and_module_from_environment(self):
        with mock.patch.dict(
            os.environ,
            {"LOGHILL_CHILD_MODULE": "worker"},
            clear=False,
        ):
            command = loghill_runner._child_command()

        self.assertEqual(command, [sys.executable, "-m", "worker"])

    def test_child_module_rejects_shell_like_whitespace(self):
        with mock.patch.dict(
            os.environ,
            {"LOGHILL_CHILD_MODULE": "worker --debug"},
            clear=False,
        ):
            with self.assertRaises(ValueError):
                loghill_runner._child_command()

    def test_forward_stream_tees_terminal_and_sends_undefined_logs(self):
        source = io.StringIO("primeira linha\n\nsegunda linha\n")
        terminal = io.StringIO()
        logger = FakeLogger()

        loghill_runner._forward_stream(source, terminal, "stderr", 1234, logger)

        self.assertEqual(terminal.getvalue(), "primeira linha\n\nsegunda linha\n")
        self.assertEqual([entry[0] for entry in logger.entries], ["primeira linha", "segunda linha"])
        self.assertTrue(all(entry[1] == "UNDEFINED" for entry in logger.entries))
        self.assertTrue(all(entry[2]["child_pid"] == 1234 for entry in logger.entries))

    def test_unix_signal_exit_code_is_normalized(self):
        self.assertEqual(loghill_runner._normalized_exit_code(-15), 143)
        self.assertEqual(loghill_runner._normalized_exit_code(2), 2)

    def test_main_supervises_child_without_network_or_real_process(self):
        logger = FakeLogger()
        child = FakeChild()
        with (
            mock.patch.object(loghill_runner, "LogHillLogger", return_value=logger),
            mock.patch.object(loghill_runner.subprocess, "Popen", return_value=child) as popen,
            mock.patch.object(loghill_runner, "_popen_options", return_value={}),
            mock.patch.object(loghill_runner.signal, "getsignal", return_value=signal.SIG_DFL),
            mock.patch.object(loghill_runner.signal, "signal"),
            mock.patch.object(loghill_runner.sys, "stdout", io.StringIO()),
            mock.patch.object(loghill_runner.sys, "stderr", io.StringIO()),
            mock.patch.dict(
                os.environ,
                {"LOGHILL_CHILD_MODULE": "examples.project_example.worker"},
                clear=False,
            ),
        ):
            exit_code = loghill_runner.main()

        self.assertEqual(exit_code, 0)
        self.assertTrue(logger.flushed)
        self.assertTrue(logger.closed)
        self.assertEqual([entry[0] for entry in logger.entries], ["saída do filho", "erro do filho"])
        command = popen.call_args.args[0]
        self.assertEqual(command, [sys.executable, "-m", "examples.project_example.worker"])
        self.assertEqual(popen.call_args.kwargs["env"]["LOGHILL_SUPERVISED"], "1")


if __name__ == "__main__":
    unittest.main()

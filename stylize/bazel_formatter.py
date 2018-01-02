from stylize.formatter import Formatter
from stylize.util import *

import os
import shutil
import subprocess
import tempfile


class BazelFormatter(Formatter):
    def __init__(self):
        super().__init__()
        self.file_extensions = [".BUILD", "BUILD", "WORKSPACE"]
        self._tempdir = tempfile.mkdtemp()

    def run(self, args, filepath, check=False, calc_diff=False):
        if check or calc_diff:
            # write style-compliant version of file to a tmp directory
            outfile_path = os.path.join(self._tempdir, filepath)
            os.makedirs(os.path.dirname(outfile_path), exist_ok=True)
            outfile = open(outfile_path, 'w')
            infile = open(filepath, 'r')
            proc = subprocess.Popen(
                ["buildifier"], stdin=infile, stdout=outfile, stderr=subprocess.PIPE)
            out, err = proc.communicate()
            outfile.close()
            infile.close()

            # return code zero indicates style-compliant file. 2 indicates non-
            # compliance.  Other return codes indicate errors.
            if proc.returncode != 0 and proc.returncode != 2:
                raise RuntimeError("Call to buildifier failed for file '%s':\n%s" %
                                   (filepath, err.decode('utf-8')))

            # note: filepath[2:] cuts off leading './'
            patch = calculate_diff(filepath, outfile_path, filepath)
            noncompliant = len(patch) > 0

            return noncompliant, patch
        else:
            logfile = open("/dev/null", "w")
            md5_before = file_md5(filepath)
            proc = subprocess.Popen(
                ["buildifier", filepath], stdout=logfile, stderr=logfile)
            proc.communicate()
            md5_after = file_md5(filepath)
            return (md5_before != md5_after), None

    def get_command(self):
        return shutil.which("buildifier")
